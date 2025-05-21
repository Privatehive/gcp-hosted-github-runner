package main

import (
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"github.com/Tereius/gcp-hosted-github-runner/pkg"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

func getEnvDefaultInt64(name string, defaultValue int64) int64 {

	if val, ok := os.LookupEnv(name); ok {
		if nb, err := strconv.Atoi(val); err == nil {
			return int64(nb)
		}
	}
	return defaultValue
}

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
		panic("Mandatory Env " + name + " not found")
	}
}

func mustBase64Decode(data string) string {

	if data, err := base64.StdEncoding.DecodeString(data); err != nil {
		panic(err)
	} else {
		return string(data)
	}
}

func main() {

	logrus.SetFormatter(&logrus.JSONFormatter{
		DisableTimestamp: true,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
	})

	if dbg := getEnvDefaultInt64("DEBUG", 0); dbg == 1 {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	config := pkg.AutoscalerConfig{
		RouteWebhook:      getEnvDefault("ROUTE_WEBHOOK", "/webhook"),
		RouteDeleteVm:     getEnvDefault("ROUTE_DELETE_VM", "/delete_vm"),
		RouteCreateVm:     getEnvDefault("ROUTE_CREATE_VM", "/create_vm"),
		ProjectId:         mustGetEnv("PROJECT_ID"),
		Zone:              mustGetEnv("ZONE"),
		TaskQueue:         mustGetEnv("TASK_QUEUE"),
		TaskTimeout:       getEnvDefaultInt64("TASK_DISPATCH_TIMEOUT", 180),
		InstanceTemplate:  mustGetEnv("INSTANCE_TEMPLATE"),
		SecretVersion:     mustGetEnv("SECRET_VERSION"),
		RunnerPrefix:      getEnvDefault("RUNNER_PREFIX", "runner"),
		RunnerGroupId:     getEnvDefaultInt64("RUNNER_GROUP_ID", 1),
		RunnerLabels:      []string{},
		RegisteredSources: map[string]pkg.Source{},
		SourceQueryParam:  getEnvDefault("SOURCE_QUERY_PARAM_NAME", "src"),
		CreateVmDelay:     getEnvDefaultInt64("CREATE_VM_DELAY", 10),
		Simulate:          getEnvDefaultInt64("SIMULATE", 0) == 1,
	}

	if enterpriseEnv := strings.Split(getEnvDefault("GITHUB_ENTERPRISE", ""), ";"); len(enterpriseEnv) == 2 {
		if _, ok := config.RegisteredSources[enterpriseEnv[0]]; !ok {
			config.RegisteredSources[enterpriseEnv[0]] = pkg.Source{
				Name:       enterpriseEnv[0],
				SourceType: pkg.TypeEnterprise,
				Secret:     mustBase64Decode(enterpriseEnv[1]),
			}
			log.Infof("Registered webhook enterprise source: %s", enterpriseEnv[0])
		} else {
			log.Warnf("Found duplicate webhook source key - will be ignored: %s", enterpriseEnv[0])
		}
	}

	if orgEnv := strings.Split(getEnvDefault("GITHUB_ORG", ""), ";"); len(orgEnv) == 2 {
		if _, ok := config.RegisteredSources[orgEnv[0]]; !ok {
			config.RegisteredSources[orgEnv[0]] = pkg.Source{
				Name:       orgEnv[0],
				SourceType: pkg.TypeOrganization,
				Secret:     mustBase64Decode(orgEnv[1]),
			}
			log.Infof("Registered webhook organization source: %s", orgEnv[0])
		} else {
			log.Warnf("Found duplicate webhook source key - will be ignored: %s", orgEnv[0])
		}
	}

	for _, repoEnv := range strings.Split(getEnvDefault("GITHUB_REPOS", ""), ",") {
		if repo := strings.Split(repoEnv, ";"); len(repo) == 2 {
			if _, ok := config.RegisteredSources[repo[0]]; !ok {
				config.RegisteredSources[repo[0]] = pkg.Source{
					Name:       repo[0],
					SourceType: pkg.TypeRepository,
					Secret:     mustBase64Decode(repo[1]),
				}
				log.Infof("Registered webhook repository source: %s", repo[0])
			} else {
				log.Warnf("Found duplicate webhook source key - will be ignored: %s", repo[0])
			}
		}
	}

	if labels := strings.Split(getEnvDefault("RUNNER_LABELS", "self-hosted"), ","); len(labels) == 0 {
		log.Warn("No workflow runner labels were provided. You should at least add the label \"self-hosted\"")
	} else {
		config.RunnerLabels = labels
	}

	if config.Simulate {
		log.Warn("Simulation mode is active - no VMs will be created/deleted")
	}

	port, _ := strconv.Atoi(getEnvDefault("PORT", "8080"))
	log.Infof("Starting autoscaler on port %d observing workflow jobs with labels \"%s\"", port, strings.Join(config.RunnerLabels, ", "))
	pkg.NewAutoscaler(config).Srv(port)
}
