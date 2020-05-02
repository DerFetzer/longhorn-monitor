package main

import (
	"context"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/derfetzer/longhorn-monitor/healthcheck/apiclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type HealthCheckConfig struct {
	Interval       uint32
	MonitorService string
}

type PodInfo struct {
	Name      string
	Namespace string
}

func initLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	log.Logger = log.With().Caller().Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func initConfig() *HealthCheckConfig {
	cfg := &HealthCheckConfig{}

	if v, p := os.LookupEnv("MONITOR_SVC"); p {
		cfg.MonitorService = v
	} else {
		log.Fatal().Msg("MONITOR_SVC environment variable has to be set")
	}

	if v, p := os.LookupEnv("INTERVAL"); p {
		if conv, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.Interval = uint32(conv)
		} else {
			log.Fatal().Err(err).Msg("INTERVAL environment variable could no be parsed")
		}
	} else {
		cfg.Interval = 60
	}

	return cfg
}

func initPodInfo() *PodInfo {
	podInfo := &PodInfo{}

	if v, p := os.LookupEnv("HOSTNAME"); p {
		podInfo.Name = v
	} else {
		log.Fatal().Msg("HOSTNAME environment variable has to be set")
	}

	if v, p := os.LookupEnv("NAMESPACE"); p {
		podInfo.Namespace = v
	} else {
		log.Fatal().Msg("NAMSPACE environment variable has to be set")
	}

	return podInfo
}

func checkPvc(result chan<- bool) {
	err := ioutil.WriteFile("/pvc/probe", []byte{0x42}, 0644)
	if err != nil {
		log.Error().Err(err).Msg("Could not write probe file")
	}
	result <- err == nil
}

func main() {
	config := initConfig()
	podInfo := initPodInfo()

	client, err := apiclient.NewClient(config.MonitorService)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create API client")
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(config.Interval) * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				result := make(chan bool, 1)
				go checkPvc(result)

				var isHealthy bool

				select {
				case res := <-result:
					isHealthy = res
				case <-time.After(2 * time.Second):
					isHealthy = false
					log.Error().Err(err).Msg("Timeout while writing probe file")
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				_, err := client.PostHealth(ctx, &apiclient.PostHealthParams{
					IsHealthy: isHealthy,
					PodName:   podInfo.Name,
					Namespace: podInfo.Namespace,
				})

				if err != nil {
					log.Error().Err(err).Msg("Could not post health to monitor")
				}

				cancel()
			}
		}
	}()

	select {
	case <-sigCh:
		log.Info().Msg("Container will be terminated")

		ticker.Stop()
		done <- true

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

		_, err := client.DeleteHealth(ctx, &apiclient.DeleteHealthParams{
			PodName:   podInfo.Name,
			Namespace: podInfo.Namespace,
		})

		if err != nil {
			log.Error().Err(err).Msg("Could not delete health from monitor")
		}

		cancel()
	}
}
