package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"runtime"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/chubchubsancho/prometheus-hydrao-exporter/internal/collector"
	"github.com/chubchubsancho/prometheus-hydrao-exporter/internal/hydrao"
)

const (
	EXPORTER = "hydrao-exporter"
)

var (
	webConfig  = kingpinflag.AddFlags(kingpin.CommandLine, ":9107")
	metricPath = kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).Envar("HYDRAO_EXPORTER_METRICS_PATH").Default("/metrics").String()
	email = kingpin.Flag(
		"hydrao.email",
		"Email address used for Hydrao account.",
	).Envar("HYDRAO_EXPORTER_ACCOUNT_EMAIL").Default("").String()
	password = kingpin.Flag(
		"hydrao.password",
		"Password used for Hydrao account.",
	).Envar("HYDRAO_EXPORTER_ACCOUNT_PASSWORD").Default("").String()
	apiKey = kingpin.Flag(
		"hydrao.api-key",
		"Api-Key for Hydrao account.",
	).Envar("HYDRAO_EXPORTER_ACCOUNT_API_KEY").String()
	maxProcs = kingpin.Flag(
		"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
	).Envar("GOMAXPROCS").Default("1").Int()
)

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print(EXPORTER))
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	cfg := hydrao.Config{
		Email:    *email,
		Password: *password,
		ApiKey:   *apiKey,
	}

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", EXPORTER), "version", version.Info())
	level.Debug(logger).Log("msg", "Build context", "build", version.BuildContext())
	level.Info(logger).Log("msg", fmt.Sprintf("Login as: %s", cfg.Email))
	client := hydrao.NewClient(cfg, logger)

	if err := client.NewSession(); err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("can't create New Hydrao session: %s", err))
	}

	if user, err := user.Current(); err == nil && user.Uid == "0" {
		level.Warn(logger).Log("msg", "Hydrao Exporter is running as root user. This exporter is designed to run as unprivileged user, root is not required.")
	}
	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	collector, err := collector.New(logger, client.GetShowerheads)
	prometheus.MustRegister(collector)

	if err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("can't create Prometheus collector: %s", err))
	}
	//defer collector.Close()

	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.Handle(*metricPath,
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer,
			promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{
					// ErrorLog: &promHTTPLogger{
					// 	logger: logger,
					// },
				},
			),
		),
	)

	server := &http.Server{}
	if err := web.ListenAndServe(server, webConfig, logger); err != nil {
		log.Fatal(err)
	}
	log.Print(collector)
	log.Print("Hello")
}
