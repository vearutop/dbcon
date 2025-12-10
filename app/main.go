// Package app provides importable main.
package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bool64/dev/version"
	"github.com/bool64/sqluct"
	_ "github.com/go-sql-driver/mysql" // DB driver.
	_ "github.com/lib/pq"              // DB driver.
	"github.com/swaggest/assertjson/json5"
	"github.com/swaggest/openapi-go/openapi31"
	"github.com/swaggest/rest/response/gzip"
	"github.com/swaggest/rest/web"
	swgui "github.com/swaggest/swgui/v5cdn"
	"github.com/swaggest/usecase"
	"github.com/vearutop/dbcon/dbcon"
	"github.com/vearutop/dbcon/internal/graceful"
	"github.com/vearutop/flatjsonl/flatjsonl"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite" // DB driver.
)

var (
	// DefaultListenAddress allows custom control.
	DefaultListenAddress = "127.0.0.1:0"

	// PrepareFlatJSONLConfig is a custom control over flatjsonl.
	PrepareFlatJSONLConfig func(cfg *flatjsonl.Config)

	// PrepareFlatJSONLFlags is a custom control over flatjsonl.
	PrepareFlatJSONLFlags func(flags *flatjsonl.Flags)
)

// Main is the main app function.
func Main() error { //nolint:funlen,cyclop,maintidx
	var (
		listen      string
		skipBrowser bool
		basicAuth   string
		tables      string
		flatconf    string
		ver         bool
	)

	flag.StringVar(&listen, "listen", DefaultListenAddress, "listen address, port 0 picks a free random port")
	flag.BoolVar(&skipBrowser, "s", false, "skip browser opening")
	flag.StringVar(&basicAuth, "auth", "", "basic auth as user:password")
	flag.StringVar(&tables, "tables", "", "comma-separated list table names to use for completion and AI")
	flag.StringVar(&flatconf, "flatconf", "", "flatjsonl config file for JSONL import")
	flag.BoolVar(&ver, "version", false, "show version")

	flag.Parse()

	if ver {
		println(version.Module("github.com/vearutop/dbcon").Version)
		return nil
	}

	if flag.NArg() == 0 {
		println("Usage of dbcon:")
		println("dbcon [OPTIONS] DB...")
		println("\tSupported DB drivers:", strings.Join(sql.Drivers(), ", "))
		println("\tDB can be a path to SQLite/CSV/JSONL file, or a URL with mysql:// or postgres:// scheme. Examples:")
		println("\t\tpostgres://user:password@localhost/dbname?sslmode=disable")
		println("\t\tmysql://user:password@localhost/dbname")
		println("\t\tsqlite:///my.db")
		println("\t\tmy.sqlite")
		println("\t\tmy2.csv")
		println("\t\tmy3.jsonl")
		println("\t\tmy4.log")
		flag.PrintDefaults()

		return nil
	}

	sh := graceful.NewSwitch(time.Second)

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	var (
		instances    []dbcon.DBInstance
		tempInstance *dbcon.DBInstance
		tempName     string
	)

	ensureTempInstance := func() error {
		if tempInstance != nil {
			return nil
		}

		tempName = path.Join(os.TempDir(), "dbcon-"+time.Now().Format("2006-01-02-15-04-05")+".sqlite")

		log.Println("using temp db instance:", tempName)

		db, err := sql.Open("sqlite", tempName)
		if err != nil {
			return fmt.Errorf("failed to open db: %w", err)
		}

		tempInstance = &dbcon.DBInstance{
			Name:     "temp",
			Dialect:  sqluct.DialectSQLite3,
			Instance: db,
		}

		instances = append(instances, *tempInstance)

		return nil
	}

	for _, dsn := range flag.Args() {
		if strings.HasSuffix(dsn, ".csv") {
			if err := ensureTempInstance(); err != nil {
				return err
			}

			if err := importCSV(tempInstance.Instance, dsn); err != nil {
				return fmt.Errorf("failed to import CSV: %w", err)
			}

			continue
		}

		fn := strings.TrimSuffix(dsn, ".zst")
		fn = strings.TrimSuffix(fn, ".gz")

		if strings.HasSuffix(fn, ".jsonl") ||
			strings.HasSuffix(fn, ".log") {
			println("importing", dsn, "as jsonl...")

			if err := ensureTempInstance(); err != nil {
				return err
			}

			f := flatjsonl.Flags{}
			f.SQLite = tempName
			f.SQLTable = strings.TrimSuffix(strings.TrimSuffix(path.Base(dsn), ".jsonl"), ".log")
			f.ProgressInterval = 5 * time.Second
			f.ChildrenLimitObject = 100
			f.SQLMaxCols = 2000
			f.Verbosity = 2
			f.Concurrency = 2 * runtime.NumCPU()
			f.MemLimit = 1000
			f.BufSize = 1e7
			if PrepareFlatJSONLFlags != nil {
				PrepareFlatJSONLFlags(&f)
			}

			cfg := flatjsonl.Config{}
			if flatconf != "" {
				if err := loadConfig(flatconf, &cfg); err != nil {
					return err
				}
			}

			if PrepareFlatJSONLConfig != nil {
				PrepareFlatJSONLConfig(&cfg)
			}

			if _, err := exec.LookPath("sqlite3"); err == nil {
				println("importing with sqlite3 CLI")

				f.SQLiteCLI = true
			}

			proc, err := flatjsonl.NewProcessor(f, cfg, flatjsonl.Input{FileName: dsn})
			if err != nil {
				return fmt.Errorf("failed to import jsonl: %w", err)
			}

			st := time.Now()

			if err := proc.Process(); err != nil {
				return fmt.Errorf("failed to process jsonl: %w", err)
			}

			println("import completed in", time.Since(st).String())

			continue
		}

		u, err := url.Parse(dsn)
		if err != nil {
			return fmt.Errorf("failed to parse dsn: %w", err)
		}

		switch {
		case strings.HasSuffix(u.Path, ".duckdb"):
			u.Scheme = "duckdb"
		case strings.HasSuffix(u.Path, ".sqlite"), strings.HasSuffix(u.Path, ".db"):
			u.Scheme = "sqlite"
		}

		switch u.Scheme {
		case "":
		case "sqlite":
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				return fmt.Errorf("failed to open db: %w", err)
			}

			instances = append(instances, dbcon.DBInstance{
				Name:     dsn,
				Dialect:  sqluct.DialectSQLite3,
				Instance: db,
			})
		case "duckdb":
			p := u.Path
			if p == "" {
				p = u.Host
			}

			db, err := sql.Open("duckdb", p)
			if err != nil {
				return fmt.Errorf("failed to open db: %w", err)
			}

			instances = append(instances, dbcon.DBInstance{
				Name:     filterDsn(dsn),
				Dialect:  sqluct.DialectUnknown,
				Instance: db,
			})
		case "postgres":
			db, err := sql.Open("postgres", dsn)
			if err != nil {
				return fmt.Errorf("failed to open db: %w", err)
			}

			instances = append(instances, dbcon.DBInstance{
				Name:     filterDsn(dsn),
				Dialect:  sqluct.DialectPostgres,
				Instance: db,
			})
		case "mysql":
			u.Host = "tcp(" + u.Host + ")"
			d2 := strings.TrimPrefix(u.String(), "mysql://")

			db, err := sql.Open("mysql", d2)
			if err != nil {
				return fmt.Errorf("failed to open db: %w", err)
			}

			instances = append(instances, dbcon.DBInstance{
				Name:     filterDsn(dsn),
				Dialect:  sqluct.DialectMySQL,
				Instance: db,
			})
		}
	}

	sh.OnShutdown("close_db", func() {
		for dsn, db := range instances {
			if err := db.Instance.Close(); err != nil {
				log.Println("failed to close db:", dsn, err.Error())
			}
		}

		if tempName != "" {
			log.Println("removing ", tempName)

			if err := os.RemoveAll(tempName); err != nil {
				log.Println(err.Error())
			}
		}
	})

	s := web.NewService(openapi31.NewReflector())

	// Init API documentation schema.
	s.OpenAPISchema().SetTitle("DB Console")
	s.OpenAPISchema().SetDescription("Database console REST API.")
	s.OpenAPISchema().SetVersion(version.Module("github.com/vearutop/dbcon").Version)

	s.Wrap(gzip.Middleware)

	if basicAuth != "" {
		s.Wrap(basicAuthMW("restricted access", basicAuth))
	}

	s.Get("/exit", usecase.NewInteractor(func(ctx context.Context, input struct{}, output *struct{}) error {
		sh.Shutdown()

		return nil
	}))

	dbcon.PrepareInstances(instances, func(o *dbcon.Options) {
		if tables != "" {
			o.TableNames = strings.Split(tables, ",")
		}
	})
	dbcon.Mount(s, "/", dbcon.DefaultDeps(instances), func(options *dbcon.Options) {
		options.AddValueProcessor("img", func(v any) any {
			if b, ok := v.([]byte); ok {
				ct := http.DetectContentType(b)

				return `<img src="data:` + ct + `;base64,` + base64.StdEncoding.EncodeToString(b) + `" />`
			}

			if s, ok := v.(string); ok {
				b, err := base64.StdEncoding.DecodeString(s)
				if err != nil {
					return err.Error()
				}

				ct := http.DetectContentType(b)

				return `<img src="data:` + ct + `;base64,` + s + `" />`
			}

			return v
		})

		options.AddValueProcessor("base36", func(v any) any {
			if b, ok := v.(int64); ok {
				return strconv.FormatInt(b, 36)
			}

			if b, ok := v.(string); ok {
				i, err := strconv.ParseInt(b, 36, 64)
				if err == nil {
					return i
				}

				return err.Error()
			}

			return v
		})
	})

	// Swagger UI endpoint at /docs.
	s.Docs("/docs", swgui.New)

	// Start server.
	srv := &http.Server{Handler: s, ReadHeaderTimeout: time.Second}

	sh.OnShutdown("http_server", func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Println("shutdown http server: ", err.Error())
		}
	})

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("failed to listen ans serve: ", err.Error())
		}
	}()

	addr := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port //nolint:errcheck

	if strings.HasPrefix(listen, ":") {
		m, err := interfaces(false)
		if err != nil {
			log.Println("find network interfaces:", err)
		} else {
			for _, v := range m {
				addr = v + ":" + strconv.Itoa(port)
			}
		}
	}

	log.Println("Console available at http://" + addr)

	if !skipBrowser {
		if err := openBrowser("http://" + addr); err != nil && !strings.Contains(err.Error(), "executable file not found") {
			log.Println("failed to open browser", err.Error())
		}
	}

	sh.Wait()

	return nil
}

func importCSV(db *sql.DB, file string) error {
	f, err := os.Open(file) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		if err := f.Close(); err != nil {
			log.Println("failed to close file:", err.Error())
		}
	}()

	r := csv.NewReader(f)

	columns, err := r.Read()
	if err != nil {
		return err
	}

	tableName := sqluct.QuoteBackticks(strings.TrimSuffix(path.Base(file), ".csv"))
	createTable := "CREATE TABLE " + tableName + " ("

	for _, column := range columns {
		createTable += sqluct.QuoteBackticks(column) + ", "
	}

	createTable = strings.TrimSuffix(createTable, ", ") + ")"

	_, err = db.Exec(createTable)
	if err != nil {
		return err
	}

	args := make([]any, len(columns))
	stmt := "INSERT INTO " + tableName + " VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")" //nolint:gosec

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	txCnt := 0

	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		for i, v := range record {
			args[i] = v
		}

		if _, err := tx.Exec(stmt, args...); err != nil {
			return err
		}

		txCnt++

		if txCnt >= 1000 {
			if err := tx.Commit(); err != nil {
				return err
			}

			txCnt = 0

			tx, err = db.Begin()
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// openBrowser opens the specified URL in the default browser of the user.
func openBrowser(url string) error {
	var (
		cmd  string
		args []string
	)

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	args = append(args, url)

	return exec.Command(cmd, args...).Start() //nolint:gosec
}

// interfaces returns a `name:ip` map of the suitable interfaces found.
func interfaces(listAll bool) ([]string, error) {
	names := make([]string, 0)

	ifaces, err := net.Interfaces()
	if err != nil {
		return names, err
	}

	re := regexp.MustCompile(`^(veth|br\-|docker|lo|EHC|XHC|bridge|gif|stf|p2p|awdl|utun|tun|tap)`)

	for _, iface := range ifaces {
		if !listAll && re.MatchString(iface.Name) {
			continue
		}

		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		ip, err := findIP(iface)
		if err != nil {
			continue
		}

		names = append(names, ip)
	}

	return names, nil
}

// FindIP returns the IP address of the passed interface, and an error.
func findIP(iface net.Interface) (string, error) {
	var ip string

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.IsLinkLocalUnicast() {
				continue
			}

			if ipnet.IP.To4() != nil {
				ip = ipnet.IP.String()

				continue
			}
			// Use IPv6 only if an IPv4 hasn't been found yet.
			// This is eventually overwritten with an IPv4, if found (see above)
			if ip == "" {
				ip = "[" + ipnet.IP.String() + "]"
			}
		}
	}

	if ip == "" {
		return "", errors.New("unable to find an IP for this interface")
	}

	return ip, nil
}

func filterDsn(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		log.Printf("filterDsn failed for %s: %s", s, err.Error())

		return s
	}

	u.User = nil

	return u.String()
}

// basicAuthMW implements a simple middleware handler for adding basic http auth to a route.
func basicAuthMW(realm string, userPass string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok {
				basicAuthFailed(w, realm)

				return
			}

			if user+":"+pass != userPass {
				basicAuthFailed(w, realm)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func basicAuthFailed(w http.ResponseWriter, realm string) {
	w.Header().Add("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
	w.WriteHeader(http.StatusUnauthorized)
}

func loadConfig(value string, cfg *flatjsonl.Config) error {
	if value == "" {
		return nil
	}

	if err := json.Unmarshal([]byte(value), cfg); err == nil {
		return nil
	}

	b, err := os.ReadFile(value)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	yerr := yaml.Unmarshal(b, cfg)
	if yerr != nil {
		err = json5.Unmarshal(b, cfg)
		if err != nil {
			return fmt.Errorf("decode config file: json5: %w, yaml: %s", err, yerr) //nolint
		}
	}

	return nil
}
