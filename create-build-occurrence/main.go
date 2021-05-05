package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/go-github/v35/github"
	collector "github.com/rode/collector-build/proto/v1alpha1"
	"github.com/sethvargo/go-envconfig"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	RunId     int64  `env:"RUN_ID,required"`
	ServerUrl string `env:"SERVER_URL,required"`
	Token     string `env:"TOKEN,required"`
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

func newGitHubClient(c *config) *github.Client {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: c.GitHub.Token,
		},
	)

	return github.NewClient(oauth2.NewClient(context.Background(), tokenSource))
}

func setOutputVariable(name, value string) {
	fmt.Printf("::set-output name=%s::%s", name, value)
}

func getRepoAndOwnerFromSlug(slug string) (string, string) {
	parts := strings.Split(slug, "/")

	return parts[0], parts[1]
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

	githubClient := newGitHubClient(&c)
	owner, repo := getRepoAndOwnerFromSlug(c.GitHub.RepoSlug)

	jobs, _, err := githubClient.Actions.ListWorkflowJobs(ctx, owner, repo, c.GitHub.RunId, &github.ListWorkflowJobsOptions{})
	if err != nil {
		logger.Fatal("error listing jobs", zap.Error(err))
	}

	var job *github.WorkflowJob
	for _, j := range jobs.Jobs {
		if j.GetName() == c.GitHub.JobId {
			job = j
			break
		}
	}
	if job == nil {
		logger.Fatal("Unable to find job")
	}

	repoUri := fmt.Sprintf("%s/%s", c.GitHub.ServerUrl, c.GitHub.RepoSlug)
	commitUri := fmt.Sprintf("%s/commit/%s", repoUri, c.GitHub.CommitId)
	logsUri := fmt.Sprintf("%s/checks/%d/logs", commitUri, job.GetID())

	request := &collector.CreateBuildRequest{
		Artifacts: []*collector.Artifact{
			{
				Id: c.ArtifactId,
			},
		},
		BuildStart:   timestamppb.New(job.GetStartedAt().Time),
		BuildEnd:     timestamppb.Now(),
		CommitId:     c.GitHub.CommitId,
		CommitUri:    commitUri,
		Creator:      c.GitHub.Actor,
		LogsUri:      logsUri,
		ProvenanceId: job.GetHTMLURL(),
		Repository:   repoUri,
	}
	logger.Info("sending request to build collector", zap.Any("request", request))
	response, err := client.CreateBuild(ctx, request)
	if err != nil {
		logger.Fatal("error creating build occurrence", zap.Error(err))
	}

	logger.Info("created occurrence", zap.Any("response", response))
	setOutputVariable("id", response.BuildOccurrenceId)
}
