package pkg

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	ginlogrus "github.com/toorop/gin-logrus"
	"google.golang.org/protobuf/proto"
)

const SHA_PREFIX = "sha256="
const SHA_HEADER = "x-hub-signature-256"
const EVENT_HEADER = "x-github-event"

type Job struct {
	Id     int64    `json:"id"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

type Payload struct {
	Action Action `json:"action"`
	Job    Job    `json:"workflow_job"`
}

type Action string

const (
	QUEUED      Action = "queued"
	COMPLETED   Action = "completed"
	IN_PROGRESS Action = "in_progress"
	WAITING     Action = "waiting"
)

type State string

const (
	// running
	PROVISIONING State = "PROVISIONING" // resources are allocated for the VM. The VM is not running yet.
	STAGING      State = "STAGING"      // resources are acquired, and the VM is preparing for first boot.
	RUNNING      State = "RUNNING"      // the VM is booting up or running.
	// stopped
	STOPPING   State = "STOPPING"   // the VM is being stopped. You requested a stop, or a failure occurred. This is a temporary status after which the VM enters the TERMINATED status.
	SUSPENDING State = "SUSPENDING" // the VM is in the process of being suspended. You suspended the VM.
	SUSPENDED  State = "SUSPENDED"  // the VM is in a suspended state. You can resume the VM or delete it.
	TERMINATED State = "TERMINATED" // the VM is stopped. You stopped the VM, or the VM encountered a failure. You can restart or delete the VM.
	// should result in running state
	REPAIRING State = "REPAIRING" // the VM is being repaired. Repairing occurs when the VM encounters an internal error or the underlying machine is unavailable due to maintenance. During this time, the VM is unusable. You are not billed when a VM is in repair. VMs are not covered by the Service level agreement (SLA) while they are in repair. If repair succeeds, the VM returns to one of the above states.
	Unknown   State = "unknown"
)

func (s State) isStopped() bool {

	return s == STOPPING || s == SUSPENDING || s == SUSPENDED || s == TERMINATED
}

func (s State) isRunning() bool {

	return s == PROVISIONING || s == STAGING || s == RUNNING || s == REPAIRING
}

type InstanceClient struct {
	*compute.InstancesClient
}

func newComputeClient() *InstanceClient {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if client, err := compute.NewInstancesRESTClient(ctx); err != nil {
		panic(err)
	} else {
		return &InstanceClient{client}
	}
}

func newTaskClient() *cloudtasks.Client {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if client, err := cloudtasks.NewClient(ctx); err != nil {
		panic(err)
	} else {
		return client
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func randStringRunes(n int) string {

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func calcSigHex(secret []byte, data []byte) string {

	sig := hmac.New(sha256.New, secret)
	sig.Write(data)
	return hex.EncodeToString(sig.Sum(nil))
}

func (s *Autoscaler) verifySignature(ctx *gin.Context) ([]byte, error) {

	if signature := ctx.GetHeader(SHA_HEADER); len(signature) == 71 {
		if body, err := io.ReadAll(ctx.Request.Body); err != nil {
			log.Errorf("Error receiving http body: %s", err.Error())
			ctx.AbortWithError(http.StatusBadRequest, err)
			return nil, err
		} else {
			if calcSignature := calcSigHex([]byte(s.conf.WebhookSecret), body); calcSignature == signature[7:] {
				return body, nil
			}
		}
	}

	log.Warnf("%s is unauthorized", ctx.RemoteIP())
	ctx.AbortWithStatus(http.StatusUnauthorized)
	return nil, fmt.Errorf("unauthorized")
}

func (s *Autoscaler) getInstanceState(ctx context.Context, instanceName string) (State, error) {

	if res, err := s.c.Get(ctx, &computepb.GetInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not get status for instance: %s - %s", instanceName, err.Error())
		return Unknown, err
	} else if res.Status == nil {
		log.Errorf("Could not read status for instance: %s", instanceName)
		return Unknown, fmt.Errorf("instance status is unknown")
	} else {
		return (State)(*res.Status), nil
	}
}

// blocking until instance started or failed to start
func (s *Autoscaler) startInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to start instance: %s", instanceName)
	if res, err := s.c.Start(ctx, &computepb.StartInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not start instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to start: %s", err.Error())
			return err
		} else {
			log.Infof("Started instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance stopped or failed to stop
func (s *Autoscaler) stopInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to stop instance: %s", instanceName)
	if res, err := s.c.Stop(ctx, &computepb.StopInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not stop instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to stop: %s", err.Error())
			return err
		} else {
			log.Infof("Stopped instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance started or failed to start
func (s *Autoscaler) deleteInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to delete instance: %s", instanceName)
	if res, err := s.c.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not delete instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to be deleted: %s", err.Error())
			return err
		} else {
			log.Infof("Deleted instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance started or failed to start
func (s *Autoscaler) createInstanceFromTemplate(ctx context.Context, instanceName string) error {

	if res, err := s.c.Insert(ctx, &computepb.InsertInstanceRequest{
		Project: s.conf.ProjectId,
		Zone:    s.conf.Zone,
		InstanceResource: &computepb.Instance{
			Name: proto.String(instanceName),
		},
		SourceInstanceTemplate: &s.conf.InstanceTemplateUrl,
	}); err != nil {
		log.Errorf("Could not create instance %s from template: %s - %s", instanceName, s.conf.InstanceTemplateUrl, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to be created from template: %s", err.Error())
			return err
		} else {
			log.Infof("Created instance from template: %s", instanceName)
		}
	}
	return nil
}

func (s *Autoscaler) createCallbackTaskWithToken(ctx context.Context, url, message string) (*taskspb.Task, error) {

	// Build the Task payload.
	// https://godoc.org/google.golang.org/genproto/googleapis/cloud/tasks/v2#CreateTaskRequest
	req := &taskspb.CreateTaskRequest{
		Parent: s.conf.TaskQueue,
		Task: &taskspb.Task{
			// https://godoc.org/google.golang.org/genproto/googleapis/cloud/tasks/v2#HttpRequest
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        url,
					Headers: map[string]string{
						SHA_HEADER: SHA_PREFIX + calcSigHex([]byte(s.conf.WebhookSecret), []byte(message)),
					},
					/*
						AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
							OidcToken: &taskspb.OidcToken{
								ServiceAccountEmail: email,
							},
						},*/
				},
			},
		},
	}

	// Add a payload message if one is present.
	req.Task.GetHttpRequest().Body = []byte(message)

	createdTask, err := s.t.CreateTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("cloudtasks.CreateTask: %w", err)
	}

	return createdTask, nil
}

func (s *Autoscaler) handleCreateRunner(ctx *gin.Context) {

	log.Info("Received handleCreateRunner call")
	if _, err := s.verifySignature(ctx); err == nil {
		runnerName := s.conf.RunnerPrefix + "-" + randStringRunes(16)
		if err := s.createInstanceFromTemplate(ctx, runnerName); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			if _, err := s.createCallbackTaskWithToken(ctx, "", "", ""); err != nil {
				log.Errorf("Immediately delete instance \"%s\" again because callback could not be created", runnerName)
				s.deleteInstance(context.Background(), runnerName) // Ignore timeous, make sure the spot instance gets destroyed
				ctx.AbortWithError(http.StatusInternalServerError, err)
			} else {
				ctx.Status(http.StatusOK)
			}
		}
	}
}

func (s *Autoscaler) handleDeleteRunner(ctx *gin.Context) {

	log.Info("Received handleDeleteRunner call")
	if data, err := s.verifySignature(ctx); err == nil {
		if err := s.deleteInstance(ctx, string(data)); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

func (s *Autoscaler) handleWebhook(ctx *gin.Context) {

	log.Info("Received webhook call")
	if data, err := s.verifySignature(ctx); err == nil {
		event := ctx.GetHeader(EVENT_HEADER)
		log.Info(ctx.Request.Header)
		log.Info(string(data))
		if event == "ping" {
			log.Info("Received ping")
			ctx.Status(http.StatusOK)
		} else if event == "workflow_job" {
			payload := Payload{}
			if err := json.Unmarshal(data, &payload); err != nil {
				log.Errorf("Can not unmarshal payload: %s", err.Error())
				ctx.AbortWithError(http.StatusBadRequest, err)
			} else {
				url := ctx.Request.URL
				if payload.Action == QUEUED {
					createUrl := url.JoinPath("../" + s.conf.RouteCreateRunner)
					log.Infof("About to create spot instance callback task with url: %s", createUrl)
					if _, err := s.createCallbackTaskWithToken(ctx, createUrl.String(), fmt.Sprint(payload.Job.Id)); err != nil {
						log.Errorf("Can not create callback: %s", err.Error())
					}
				} else if payload.Action == COMPLETED {
					delteUrl := url.JoinPath("../" + s.conf.RouteDeleteRunner)
					log.Infof("About to create spot instance delete callback task with url: %s", delteUrl)
				}
				ctx.Status(http.StatusOK)
			}
		} else {
			log.Warnf("Unknown GitHub event \"%s\" received - ignored", event)
			ctx.Status(http.StatusOK)
		}
	}
}

type AutoscalerConfig struct {
	RouteCreateRunner   string
	RouteDeleteRunner   string
	RouteWebhook        string
	WebhookSecret       string
	ProjectId           string
	Zone                string
	TaskQueue           string
	InstanceTemplateUrl string
	RunnerPrefix        string
}

type Autoscaler struct {
	engine *gin.Engine
	c      *InstanceClient
	t      *cloudtasks.Client
	conf   AutoscalerConfig
}

func NewAutoscaler(config AutoscalerConfig) Autoscaler {

	engine := gin.New()
	scaler := Autoscaler{
		engine: engine,
		c:      newComputeClient(),
		t:      newTaskClient(),
		conf:   config,
	}
	engine.Use(ginlogrus.Logger(log.WithFields(log.Fields{})))
	engine.POST(config.RouteCreateRunner, scaler.handleCreateRunner)
	engine.POST(config.RouteDeleteRunner, scaler.handleDeleteRunner)
	engine.POST(config.RouteWebhook, scaler.handleWebhook)
	engine.GET("/healthcheck", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	return scaler
}

func (s *Autoscaler) Srv(port int) {

	s.engine.Run(fmt.Sprintf("0.0.0.0:%d", port))
}
