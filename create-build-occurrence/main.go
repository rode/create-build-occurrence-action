package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	collector "github.com/rode/collector-build/proto/v1alpha1"
	"github.com/sethvargo/go-envconfig"
	"go.uber.org/zap"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type buildCollectorConfig struct {
	Host     string `env:"HOST,required"`
	Insecure bool   `env:"INSECURE"`
}

type githubConfig struct {
	Actor     string `env:"ACTOR,required"`
	CommitId  string `env:"SHA,required"`
	JobId     string `env:"JOB,required"`
	RepoSlug  string `env:"REPOSITORY,required"`
	RunId     int `env:"RUN_ID,required"`
	ServerUrl string `env:"SERVER_URL,required"`
}

type config struct {
	ArtifactId     string                `env:"ARTIFACT_ID,required"`
	BuildCollector *buildCollectorConfig `env:",prefix=BUILD_COLLECTOR_"`
	Debug          bool                  `env:"DEBUG"`
	GitHub         *githubConfig         `env:",prefix=GITHUB_"`
}

func newBuildCollectorClient(c *config) (*grpc.ClientConn, collector.BuildCollectorClient) {
	dialOptions := []grpc.DialOption{
		grpc.WithBlock(),
	}
	if c.BuildCollector.Insecure {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	conn, err := grpc.DialContext(ctx, c.BuildCollector.Host, dialOptions...)
	if err != nil {
		log.Fatal(err)
	}

	return conn, collector.NewBuildCollectorClient(conn)
}

func newLogger(c *config) (*zap.Logger, error) {
	loggerConfig := zap.NewDevelopmentConfig()

	if !c.Debug {
		loggerConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return loggerConfig.Build()
}

func setOutputVariable(name, value string) {
	fmt.Printf("::set-output name=%s::%s", name, value)
}

func main() {
	ctx := context.Background()
	var c config
	if err := envconfig.Process(ctx, &c); err != nil {
		log.Fatalf("unable to build config: %s", err)
	}

	logger, err := newLogger(&c)
	if err != nil {
		log.Fatalf("failed to create logger: %s", err)
	}

	conn, client := newBuildCollectorClient(&c)
	defer conn.Close()

	logger.Debug("start", zap.Any("config", c))

	repoUrl := fmt.Sprintf("%s/%s", c.GitHub.ServerUrl, c.GitHub.RepoSlug)
	workflowId := fmt.Sprintf("%s/actions/runs/%d", repoUrl, c.GitHub.RunId)
	logsUri := fmt.Sprintf("%s/commit/%s/checks/%s/logs", repoUrl, c.GitHub.CommitId, c.GitHub.JobId)

	request := &collector.CreateBuildRequest{
		Repository: repoUrl,
		Artifacts: []*collector.Artifact{
			{
				Id: c.ArtifactId,
			},
		},
		CommitId:     c.GitHub.CommitId,
		ProvenanceId: workflowId,
		LogsUri:      logsUri,
		Creator:      c.GitHub.Actor,
		//BuildStart:   nil,
		//BuildEnd:     nil,
	}
	logger.Info("sending request to build collector", zap.Any("request", request))
	response, err := client.CreateBuild(ctx, request)
	if err != nil {
		logger.Fatal("error creating build occurrence", zap.Error(err))
	}

	logger.Info("created occurrence", zap.Any("response", response))
	setOutputVariable("id", response.BuildOccurrenceId)
}
