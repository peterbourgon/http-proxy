package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/oklog/run"
	"github.com/pkg/errors"
)

func main() {
	fs := flag.NewFlagSet("http-proxy", flag.ExitOnError)
	var (
		httpAddr = fs.String("http", ":80", "serve HTTP on this address (optional)")
		tlsAddr  = fs.String("tls", "", "serve TLS on this address (optional)")
		cert     = fs.String("cert", "server.crt", "TLS certificate")
		key      = fs.String("key", "server.key", "TLS key")
		config   = fs.String("config", "proxy.conf", "config file")
		example  = fs.Bool("example", false, "print example config file to stdout and exit")
	)
	fs.Usage = usageFor(fs, "http-proxy [flags]")
	fs.Parse(os.Args[1:])

	if *example {
		fmt.Fprintf(os.Stdout, "example.com, www.example.com: 8081\n")
		fmt.Fprintf(os.Stdout, "subdomain.example.com: 10001\n")
		fmt.Fprintf(os.Stdout, "www.website.online: /var/www/website.online\n")
		os.Exit(0)
	}

	var cfgmap atomic.Value
	{
		cfg, err := loadcfg(*config)
		if err != nil {
			log.Fatal(err)
		}
		cfgmap.Store(cfg)
	}

	var handler http.Handler
	{
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := cfgmap.Load().(configuration)
			proxy, ok := cfg[r.Host]
			if !ok {
				log.Printf("%s %s -> not configured", r.RemoteAddr, r.Host)
				http.NotFound(w, r)
				return
			}
			log.Printf("%s %s -> %s", r.RemoteAddr, r.Host, proxy.dest)
			proxy.ServeHTTP(w, r)
		})
	}

	var g run.Group
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGHUP)
			for {
				select {
				case <-c:
					log.Printf("received SIGHUP, reloading config...")
					cfg, err := loadcfg(*config)
					if err != nil {
						log.Printf("bad config, ignoring (%v)", err)
						continue
					}
					cfgmap.Store(cfg)
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}, func(error) {
			cancel()
		})
	}
	{
		if *tlsAddr != "" {
			server := &http.Server{Addr: *tlsAddr, Handler: hsts(handler)}
			g.Add(func() error {
				log.Printf("serving TLS on %s", *tlsAddr)
				return server.ListenAndServeTLS(*cert, *key)
			}, func(error) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				server.Shutdown(ctx)
			})
		} else {
			log.Printf("not serving TLS")
		}
	}
	{
		if *httpAddr != "" {
			server := &http.Server{Addr: *httpAddr, Handler: handler}
			g.Add(func() error {
				log.Printf("serving HTTP on %s", *httpAddr)
				return server.ListenAndServe()
			}, func(error) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				server.Shutdown(ctx)
			})
		} else {
			log.Printf("not serving HTTP")
		}
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				return fmt.Errorf("received signal %s", sig)
			case <-ctx.Done():
				return ctx.Err()
			}
		}, func(error) {
			cancel()
		})
	}
	log.Printf("exit: %v", g.Run())
}

type configuration map[string]target

type target struct {
	dest string
	http.Handler
}

func loadcfg(filename string) (configuration, error) {
	f, err := os.Open(filename)
	if err != nil {
		return configuration{}, errors.Wrap(err, "Open failed")
	}
	defer f.Close()

	var (
		cfg = configuration{}
		s   = bufio.NewScanner(f)
	)
	for s.Scan() {
		toks := strings.SplitN(s.Text(), ":", 2)
		if len(toks) != 2 {
			return cfg, errors.Errorf("bad line: %s", s.Text())
		}
		var (
			hosts, dest = toks[0], strings.TrimSpace(toks[1])
			handler     http.Handler
		)
		if _, err := strconv.Atoi(dest); err == nil {
			hostport := net.JoinHostPort("127.0.0.1", dest)
			u := &url.URL{Scheme: "http", Host: hostport}
			handler = httputil.NewSingleHostReverseProxy(u)
		} else if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
			handler = http.FileServer(http.Dir(dest))
		} else {
			return cfg, errors.Errorf("invalid proxy target: %s", s.Text())
		}
		for _, host := range strings.Split(hosts, ",") {
			src := strings.TrimSpace(host)
			log.Printf("loadcfg %s -> %s", src, dest)
			cfg[src] = target{dest: dest, Handler: handler}
		}
	}

	return cfg, nil
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stdout, "USAGE\n")
		fmt.Fprintf(os.Stdout, "  %s\n", short)
		fmt.Fprintf(os.Stdout, "\n")
		fmt.Fprintf(os.Stdout, "FLAGS\n")
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			def := f.DefValue
			if def == "" {
				def = "..."
			}
			fmt.Fprintf(tw, "  -%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		tw.Flush()
	}
}

func hsts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
