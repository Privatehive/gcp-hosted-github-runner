package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/Tereius/gcp-hosted-github-runner/pkg"
	"github.com/sirupsen/logrus"
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

	logrus.SetFormatter(&logrus.JSONFormatter{
		DisableTimestamp: true,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyLevel: "severity",
		},
	})
	logrus.SetLevel(logrus.InfoLevel)

	labels := strings.Split(getEnvDefault("RUNNER_LABELS", "self-hosted"), ",")
	runnerGroup := getEnvDefault("RUNNER_GROUP_NAME", "Default")
	runnerGroupId, _ := strconv.Atoi(getEnvDefault("RUNNER_GROUP_ID", "0"))
	scaler := pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteWebhook:     getEnvDefault("ROUTE_WEBHOOK", "/webhook"),
		RouteDeleteVm:    getEnvDefault("ROUTE_DELETE_VM", "/delete_vm"),
		RouteCreateVm:    getEnvDefault("ROUTE_CREATE_VM", "/create_vm"),
		WebhookSecret:    getEnvDefault("WEBHOOK_SECRET", ""),
		ProjectId:        mustGetEnv("PROJECT_ID"),
		Zone:             mustGetEnv("ZONE"),
		TaskQueue:        mustGetEnv("TASK_QUEUE"),
		InstanceTemplate: mustGetEnv("INSTANCE_TEMPLATE"),
		SecretVersion:    mustGetEnv("SECRET_VERSION"),
		RunnerPrefix:     getEnvDefault("RUNNER_PREFIX", "runner"),
		RunnerGroupName:  runnerGroup,
		RunnerGroupId:    runnerGroupId,
		RunnerLabels:     labels,
		GitHubOrg:        mustGetEnv("GITHUB_ORG"),
	})

	if len(labels) == 0 {
		log.Warn("No workflow runner labels were provided. You should at least add the label \"self-hosted\"")
	}

	port, _ := strconv.Atoi(getEnvDefault("PORT", "8080"))
	log.Infof("Starting autoscaler on port %d observing workflow jobs of runner group \"%s\" with labels \"%s\"", port, runnerGroup, strings.Join(labels, ", "))
	scaler.Srv(port)
}
