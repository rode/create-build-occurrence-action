# create-build-occurrence-action

A GitHub Action to create a build occurrence during a workflow run.
It's intended to be used along with [Rode](https://github.com/rode/rode) and an instance of the [build collector](https://github.com/rode/collector-build).

## Use

```yaml
  - name: Create Build Occurrence
    uses: rode/create-build-occurrence-action@v0.1.0
    with:
      artifactId: harbor.example.com/rode-demo/rode-demo-node-app@${{ steps.build.outputs.digest }}
      artifactNames: |
        harbor.example.com/rode-demo/rode-demo-node-app:v1.2.3
      buildCollectorHost: ${{ env.BUILD_COLLECTOR_HOST }}
      githubToken: ${{ secrets.GITHUB_TOKEN }}
```

### Inputs

| Input                    | Description                                                                                  | Default |
|--------------------------|----------------------------------------------------------------------------------------------|---------|
| `artifactId`             | The identifier of the created artifact                                                       | N/A     |
| `artifactNames`          | A list of alternative names for the artifact. If using Docker, these are any additional tags | `""`    |
| `artifactNamesDelimiter` | Used to separate artifactNames                                                               | `\n`    |
| `buildCollectorHost`     | The build collector hostname                                                                 | N/A     |
| `buildCollectorInsecure` | When set, the connection to the build collector will not use TLS                             | `false` |
| `githubToken`            | GitHub token used to pull information about the workflow and job                             | N/A     |

### Outputs

| Output | Description                                       |
|--------|---------------------------------------------------|
| `id`   | The unique identifier of the new build occurrence |

## Local Development

1. Configuration for the action is sourced from the environment, the easiest way to run locally is to set the following environment variables,
or place them in a file called `.env`:
    ```
    ARTIFACT_ID="test.foo@sha256:123"
    
    BUILD_COLLECTOR_HOST=rode-collector-build.rode-demo.svc.cluster.local:8082
    BUILD_COLLECTOR_INSECURE=true
    
    GITHUB_ACTOR="foo@example.com"
    GITHUB_SHA="hash"
    GITHUB_JOB=job-name
    GITHUB_RUN_ID=1234
    GITHUB_SERVER_URL='https://github.com'
    GITHUB_TOKEN='topsecret'
    GITHUB_REPOSITORY=rode/demo-app
    ```
1. Then `env $(cat .env | xargs) go run action.go main.go` or simply `go run` if the variables are already set
1. Update any formatting issues with `make fmt`
1. Run the tests with `make test`
