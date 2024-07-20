package main

import (
	"os"

	"github.com/Tereius/g-spot-runner-github-actions/pkg"
	log "github.com/sirupsen/logrus"
)

func getEnvDefault(name string, defaultValue string) string {

	if val, ok := os.LookupEnv(name); ok {
		return val
	}
	return defaultValue
}

func mustGetEnv(name string) string {

	if val, ok := os.LookupEnv(name); ok {
		return val
	} else {
		panic("Env " + name + " not found")
	}
}

func main() {

	log.Info("Starting poll server")

	scaler := pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteCreateRunner:   getEnvDefault("ROUTE_CREATE_RUNNER", "/create_runner"),
		RouteDeleteRunner:   getEnvDefault("ROUTE_DELETE_RUNNER", "/delete_runner"),
		RouteWebhook:        getEnvDefault("ROUTE_WEBHOOK", "/webhook"),
		WebhookSecret:       getEnvDefault("WEBHOOK_SECRET", "arbitrarySecret"),
		ProjectId:           mustGetEnv("PROJECT_ID"),
		Zone:                mustGetEnv("ZONE"),
		TaskQueue:           mustGetEnv("TASK_QUEUE"),
		InstanceTemplateUrl: mustGetEnv("INSTANCE_TEMPLATE_URL"),
		RunnerPrefix:        getEnvDefault("RUNNER_PREFIX", "runner"),
	})
	scaler.Srv(8080)
}
