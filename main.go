package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v35/github"
	collector "github.com/rode/collector-build/proto/v1alpha1"
	"github.com/sethvargo/go-envconfig"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
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
	ArtifactId             string                `env:"ARTIFACT_ID,required"`
	ArtifactNames          string                `env:"ARTIFACT_NAMES"`
	ArtifactNamesDelimiter string                `env:"ARTIFACT_NAMES_DELIMITER,required"`
	BuildCollector         *buildCollectorConfig `env:",prefix=BUILD_COLLECTOR_"`
	Debug                  bool                  `env:"DEBUG"`
	GitHub                 *githubConfig         `env:",prefix=GITHUB_"`
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

func fatal(message string) {
	fmt.Println(message)
	os.Exit(1)
}

func main() {
	ctx := context.Background()
	var c *config
	if err := envconfig.Process(ctx, c); err != nil {
		fatal(fmt.Sprintf("unable to build config: %s", err))
	}

	logger, err := newLogger(c)
	if err != nil {
		fatal(fmt.Sprintf("failed to create logger: %s", err))
	}

	conn, client := newBuildCollectorClient(c)
	defer conn.Close()

	githubClient := newGitHubClient(c)

	action := &createBuildOccurrenceAction{
		actions: githubClient.Actions,
		config:  c,
		client:  client,
		logger:  logger,
	}

	occurrenceId, err := action.Run(ctx)
	if err != nil {
		fatal(err.Error())
	}

	setOutputVariable("id", occurrenceId)
}
