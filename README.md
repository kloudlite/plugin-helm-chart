# plugin-helm-chart

This is a Kubernetes Operator for declarative Helm Chart management, part of the Kloudlite platform. It provides Custom Resource Definitions (CRDs) to manage Helm deployments as Kubernetes resources.

## Description

The plugin-helm-chart operator enables declarative management of Helm charts within Kubernetes clusters. It provides two main custom resources:
- **HelmChart**: For managing individual Helm chart deployments
- **HelmPipeline**: For orchestrating multiple HelmChart resources in sequence

The operator executes Helm operations securely through Kubernetes Jobs, integrating seamlessly with Kloudlite's plugin system for comprehensive service management.

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### Development

#### Using Makefile (standard Kubebuilder commands)
```bash
# Generate code after API changes (ALWAYS run both together)
make manifests generate

# Run tests
make test              # Unit tests with envtest
make test-e2e          # E2E tests (requires Kind cluster)

# Lint code
make lint              # Check for issues
make lint-fix          # Auto-fix issues

# Run controller locally
make run

# CRD management
make install           # Install CRDs to cluster
make uninstall         # Remove CRDs from cluster

# Deployment
make deploy IMG=<registry>/<image>:<tag>
make undeploy
```

#### Using Runfile.yml (preferred for development)
```bash
# Run controller with environment variables from .secrets/env
run dev

# Create new API resource
run new:api version=v1 kind=YourResourceName

# Generate manifests and code (wrapper for make commands)
run manifests

# Build operations
run build                                    # Build binary
run image:build image=<registry>/<image>     # Multi-arch build
run helm-job-runner tag=<version>            # Build helm job runner image
```

#### Running specific tests
```bash
# Run tests for a specific controller
go test ./internal/controller/helm_chart/... -v

# Run specific test case with Ginkgo
go test ./internal/controller/helm_chart/... -v -ginkgo.focus="should create a Job"

# Run tests with coverage
go test ./... -coverprofile cover.out
```

### Building and Deployment

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/plugin-helm-chart:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/plugin-helm-chart:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Creating New APIs
```bash
# Create a new API resource (uses kubebuilder)
run new:api version=v1 kind=YourResourceName
```

## Architecture

### Custom Resources

1. **HelmChart** - Manages individual Helm chart deployments
   - Controller: `/internal/controller/helm_chart/helmchart_controller.go`
   - API: `/api/v1/helmchart_types.go`
   - Executes Helm operations via Kubernetes Jobs

2. **HelmPipeline** - Orchestrates multiple HelmChart resources in sequence
   - Controller: `/internal/controller/helm_pipeline/controller.go`
   - API: `/api/v1/helmpipeline_types.go`
   - Creates and manages HelmChart resources for each pipeline step

### Key Design Patterns

- **Job-based Execution**: Helm operations run in Kubernetes Jobs using the helm-job-runner image
- **Security**: Jobs run with hardened security contexts (non-root, read-only filesystem)
- **Plugin Integration**: Integrates with Kloudlite's plugin system for service management
- **Status Tracking**: Uses controller-runtime's status subresource for operation tracking

### Controller Reconciliation Pattern

Both controllers follow the same reconciliation pattern:
1. Check resource deletion (handle finalizers)
2. Create ServiceAccount, Role, RoleBinding for Job execution
3. Create/update Job from embedded templates
4. Monitor Job status and update resource status accordingly
5. Set appropriate conditions on the resource

### Important Environment Variables

```bash
# Required for local development (set in .secrets/env)
HELM_JOB_IMAGE=ghcr.io/kloudlite/kloudlite/operator/workers/helm-job-runner:latest

# Optional for tests
PROMETHEUS_INSTALL_SKIP=true      # Skip Prometheus in e2e tests
CERT_MANAGER_INSTALL_SKIP=true    # Skip cert-manager in e2e tests
```

### Testing Strategy

#### Unit Tests
- Use Ginkgo v2 + Gomega for BDD-style tests
- Use envtest for simulated Kubernetes API
- Tests located next to controllers (e.g., `helmchart_controller_test.go`)
- Test helpers available in `internal/controller/test_helpers.go`

#### Test Helper Functions
- `WaitForJob()` - Wait for job creation with specific labels
- `SimulateJobCompletion()` - Mark job as completed
- `SimulateJobFailure()` - Mark job as failed
- `CreateHelmChartWithDefaults()` - Create test HelmChart with sensible defaults
- `CreateHelmPipelineWithDefaults()` - Create test HelmPipeline with sensible defaults
- `WaitForHelmChartReady()` - Wait for HelmChart to be ready
- `WaitForHelmPipelineReady()` - Wait for HelmPipeline to be ready
- `CleanupNamespace()` - Clean all test resources from namespace

#### Writing Tests Example
```go
// Example test structure
var _ = Describe("Controller", func() {
    Context("when creating a HelmChart", func() {
        It("should create a Job", func() {
            // Create resource
            hc := testHelpers.CreateHelmChartWithDefaults("test", namespace)
            Expect(k8sClient.Create(ctx, hc)).To(Succeed())
            
            // Wait for job
            job, err := testHelpers.WaitForJob(namespace, labels, timeout)
            Expect(err).NotTo(HaveOccurred())
            
            // Simulate completion
            Expect(testHelpers.SimulateJobCompletion(job)).To(Succeed())
            
            // Verify status
            Expect(testHelpers.WaitForHelmChartReady(name, namespace, timeout)).To(Succeed())
        })
    })
})
```

## Common Workflows

### Adding a New Field to a CRD
1. Update the types in `/api/v1/` (e.g., `helmchart_types.go`)
2. Run `make manifests generate` to update CRDs and generated code
3. Update the controller logic to handle the new field
4. Add tests for the new functionality

### Debugging Failed Helm Operations
1. Check the HelmChart status: `kubectl get helmchart -n <namespace> <name> -o yaml`
2. Look for the Job created by the operator: `kubectl get jobs -n <namespace>`
3. Check Job logs: `kubectl logs -n <namespace> job/<job-name>`
4. Common issues:
   - Missing RBAC permissions
   - Invalid Helm values
   - Chart URL/version issues

### Updating Helm Job Runner Image
1. Make changes in `IMAGES/helm-job-runner/`
2. Build: `run helm-job-runner tag=<new-version>`
3. Update default in `cmd/main.go` or set via env var

### Multi-Architecture Builds
The project supports both amd64 and arm64 architectures. CI automatically builds for both platforms using GitHub Actions.

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/plugin-helm-chart:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/plugin-helm-chart/<tag or branch>/dist/install.yaml
```

## Important Design Patterns

### Status Management
- Use `reconciler.Request` from kloudlite toolkit
- Set conditions: Ready, JobCreated, JobCompleted
- Always update LastObservedGeneration
- Handle transient errors with requeue

### RBAC for Jobs
- Each HelmChart gets its own ServiceAccount
- Minimal permissions based on release namespace
- Automatic cleanup with owner references

### Template Rendering
- Templates use Go text/template
- Access to full HelmChart/Pipeline spec
- Environment variables passed to containers
- Security contexts are hardened by default
- Template files embedded in binary using `go:embed`

### Project Structure
```
api/v1/                           # CRD API definitions
├── helmchart_types.go           # HelmChart CRD
└── helmpipeline_types.go        # HelmPipeline CRD

internal/controller/
├── helm_chart/                  # HelmChart controller
│   ├── helmchart_controller.go  # Main reconciliation logic
│   └── templates/               # Job templates for helm operations
├── helm_pipeline/               # HelmPipeline controller
│   ├── controller.go            # Pipeline orchestration logic
│   └── templates/               # Pipeline-specific templates
└── test_helpers.go              # Shared test utilities
```

## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
