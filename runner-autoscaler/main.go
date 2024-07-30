package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/Tereius/gcp-hosted-github-runner/pkg"
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

	labels := strings.Split(getEnvDefault("RUNNER_LABELS", "self-hosted"), ",")
	runnerGroup := getEnvDefault("RUNNER_GROUP", "Default")
	scaler := pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteWebhook:     getEnvDefault("ROUTE_WEBHOOK", "/webhook"),
		RouteCreateVm:    getEnvDefault("ROUTE_CREATE_VM", "/create_vm"),
		RouteDeleteVm:    getEnvDefault("ROUTE_DELETE_VM", "/delete_vm"),
		WebhookSecret:    getEnvDefault("WEBHOOK_SECRET", ""),
		ProjectId:        mustGetEnv("PROJECT_ID"),
		Zone:             mustGetEnv("ZONE"),
		TaskQueue:        mustGetEnv("TASK_QUEUE"),
		InstanceTemplate: mustGetEnv("INSTANCE_TEMPLATE"),
		RunnerPrefix:     getEnvDefault("RUNNER_PREFIX", "runner"),
		RunnerGroup:      runnerGroup,
		RunnerLabels:     labels,
	})

	if len(labels) == 0 {
		log.Warn("No workflow runner labels were provided. You should at least add the label \"self-hosted\"")
	}

	port, _ := strconv.Atoi(getEnvDefault("PORT", "8080"))
	log.Infof("Starting autoscaler on port %d observing workflow jobs of runner group \"%s\" with labels \"%s\"", port, runnerGroup, strings.Join(labels, ", "))
	scaler.Srv(port)
}
