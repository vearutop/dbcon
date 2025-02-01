// Package app provides importable main.
package app

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/bool64/dev/version"
	_ "github.com/go-sql-driver/mysql" // DB driver.
	_ "github.com/lib/pq"              // DB driver.
	"github.com/swaggest/openapi-go/openapi31"
	"github.com/swaggest/rest/response/gzip"
	"github.com/swaggest/rest/web"
	swgui "github.com/swaggest/swgui/v5cdn"
	"github.com/swaggest/usecase"
	"github.com/vearutop/dbcon/dbcon"
	"github.com/vearutop/dbcon/internal/graceful"
	_ "modernc.org/sqlite" // DB driver.
)

// Main is the main app function.
func Main() { //nolint:funlen,cyclop
	var (
		listen      string
		skipBrowser bool
	)

	flag.StringVar(&listen, "listen", "127.0.0.1:0", "listen address, port 0 picks a free random port")
	flag.BoolVar(&skipBrowser, "s", false, "skip browser opening")

	flag.Parse()

	if flag.NArg() == 0 {
		println("Usage of dbcon:")
		println("dbcon [OPTIONS] DB...")
		println("\tDB can be a path to SQLite file, or a URL with mysql:// or postgres:// scheme. Examples:")
		println("\t\tpostgres://user:password@localhost/dbname?sslmode=disable")
		println("\t\tmysql://user:password@localhost/dbname")
		println("\t\tsqlite:///my.db")
		println("\t\tmy.sqlite")
		flag.PrintDefaults()

		return
	}

	sh := graceful.NewSwitch(time.Second)

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		log.Println("failed to start server:", err.Error())

		return
	}

	instances := map[string]*sql.DB{}
	completions := map[string][]dbcon.SQLCompletion{}

	for _, dsn := range flag.Args() {
		u, err := url.Parse(dsn)
		if err != nil {
			log.Println("failed to parse dsn:", err.Error())

			return
		}

		switch u.Scheme {
		case "":
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				log.Println("failed to open db:", err.Error())

				return
			}

			println("opened db:", dsn)
			instances[dsn] = db
			completions[dsn] = dbcon.SqliteCompletions(db)
		case "postgres":
			db, err := sql.Open("postgres", dsn)
			if err != nil {
				log.Println("failed to open db:", err.Error())

				return
			}

			instances[dsn] = db
		case "mysql":
			u.Host = "tcp(" + u.Host + ")"
			d2 := strings.TrimPrefix(u.String(), "mysql://")

			db, err := sql.Open("mysql", d2)
			if err != nil {
				log.Println("failed to open db:", err.Error())

				return
			}

			instances[dsn] = db
		}
	}

	sh.OnShutdown("close_db", func() {
		for dsn, db := range instances {
			if err := db.Close(); err != nil {
				log.Println("failed to close db:", dsn, err.Error())
			}
		}
	})

	s := web.NewService(openapi31.NewReflector())

	// Init API documentation schema.
	s.OpenAPISchema().SetTitle("DB Console")
	s.OpenAPISchema().SetDescription("Database console REST API.")
	s.OpenAPISchema().SetVersion(version.Module("github.com/vearutop/dbcon").Version)

	s.Wrap(gzip.Middleware)

	s.Get("/exit", usecase.NewInteractor(func(ctx context.Context, input struct{}, output *struct{}) error {
		sh.Shutdown()

		return nil
	}))

	dbcon.Mount(s, "/", dbcon.DefaultDeps(instances, completions))

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

	if strings.HasPrefix(listen, ":") {
		m, err := interfaces(false)
		if err != nil {
			log.Println("find network interfaces:", err)
		} else {
			for _, v := range m {
				addr = v + listen
			}
		}
	}

	log.Println("http://" + addr)

	if !skipBrowser {
		if err := openBrowser("http://" + addr); err != nil && !strings.Contains(err.Error(), "executable file not found") {
			log.Println("failed to open browser", err.Error())
		}
	}

	sh.Wait()
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
