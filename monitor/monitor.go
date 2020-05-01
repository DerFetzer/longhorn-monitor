package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/deepmap/oapi-codegen/pkg/middleware"
	"github.com/derfetzer/longhorn-monitor/monitor/apiserver"

	gommonlog "github.com/labstack/gommon/log"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v2"

	"github.com/labstack/echo/v4"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type MonitorConfig struct {
	RestartThreshold uint32
	Debug            bool
}

func initConfig() *MonitorConfig {
	cfg := &MonitorConfig{}

	restartThreshold := os.Getenv("RESTART_THRESHOLD")
	if v, err := strconv.ParseUint(restartThreshold, 10, 32); err == nil {
		cfg.RestartThreshold = uint32(v)
	} else {
		log.Fatal().Err(err).Msg("RESTART_THRESHOLD environment variable could not be parsed")
	}

	debug := os.Getenv("DEBUG")
	if v, err := strconv.ParseBool(debug); err == nil {
		cfg.Debug = v
	} else {
		cfg.Debug = false
	}

	return cfg
}

func initLogging(config *MonitorConfig) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	log.Logger = log.With().Caller().Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if config.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

func initKubernetes() kubernetes.Interface {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create in-cluster config")
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create clientset")
	}

	return clientset
}

func initWebServer(healthMonitor *apiserver.HealthMonitor) *echo.Echo {
	swagger, err := apiserver.GetSwagger()
	if err != nil {
		log.Fatal().Err(err).Msg("Error loading swagger spec")
	}

	// Clear out the servers array in the swagger spec, that skips validating
	// that server names match. We don't know how this thing will be run.
	swagger.Servers = nil

	// This is how you set up a basic Echo router
	e := echo.New()

	logger := lecho.New(
		os.Stderr,
		lecho.WithLevel(gommonlog.INFO),
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)
	e.Logger = logger

	// Log all requests
	e.Use(lecho.Middleware(lecho.Config{Logger: logger}))
	// Use our validation middleware to check all requests against the
	// OpenAPI schema.
	e.Use(middleware.OapiRequestValidator(swagger))

	// We now register our healthMonitor above as the handler for the interface
	apiserver.RegisterHandlers(e, healthMonitor)

	return e
}

func initHealthMonitor(podDeletes chan<- apiserver.PodIdentifier, deleteResults <-chan apiserver.PodDeleteResult, config *MonitorConfig) *apiserver.HealthMonitor {
	return apiserver.NewHealthMonitor(podDeletes, deleteResults, config.RestartThreshold)
}

func deletePod(podDeletes <-chan apiserver.PodIdentifier, deleteResults chan<- apiserver.PodDeleteResult, clientset kubernetes.Interface) {
	for {
		podIdentifier := <-podDeletes
		podClient := clientset.CoreV1().Pods(podIdentifier.Namespace)
		err := podClient.Delete(podIdentifier.Name, &metav1.DeleteOptions{})
		if errors.IsNotFound(err) {
			log.Warn().
				Interface("podIdentifier", podIdentifier).
				Msg("Pod not found")
		} else if err != nil {
			log.Error().
				Err(err).
				Interface("podIdentifier", podIdentifier).
				Msg("Error deleting pod")
		} else {
			log.Info().
				Interface("podIdentifier", podIdentifier).
				Msg("Pod deleted")
			deleteResults <- apiserver.PodDeleteResult{Identifier: podIdentifier, Success: true}
			continue
		}
		deleteResults <- apiserver.PodDeleteResult{Identifier: podIdentifier, Success: false}
	}
}

func main() {
	var port = flag.Int("port", 8080, "Port for HTTP server")
	flag.Parse()

	config := initConfig()
	initLogging(config)
	clientset := initKubernetes()
	podDeletes := make(chan apiserver.PodIdentifier)
	deleteResults := make(chan apiserver.PodDeleteResult)
	healthMonitor := initHealthMonitor(podDeletes, deleteResults, config)
	e := initWebServer(healthMonitor)

	if config.Debug {
		e.Debug = true
	}

	go deletePod(podDeletes, deleteResults, clientset)

	// And we serve HTTP until the world ends.
	e.Logger.Fatal(e.Start(fmt.Sprintf("0.0.0.0:%d", *port)))
}
