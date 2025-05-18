{{- with .}}
{{- /* gotype: github.com/kloudlite/plugin-helm-chart/internal/controller/templates.InstallJobVars */ -}}
apiVersion: batch/v1
kind: Job
{{- /* metadata: {{.Metadata | toJson }} */}}
metadata:
  name:  {{.Metadata.Name | toJson }}
  namespace: {{ .Metadata.Namespace | toJson }}
  labels: {{.Metadata.Labels | toJson }}
  annotations: {{.Metadata.Annotations | toJson }}
  ownerReferences: {{ .Metadata.OwnerReferences | default list | toJson }}
spec:
  template:
    metadata:
      annotations: {{.ObservabilityAnnotations | default dict | toJson }}
    spec:
      serviceAccountName: {{.ServiceAccountName | toJson }}
      tolerations: {{ .Tolerations | default list | toJson }}
      affinity: {{ .Affinity | default dict | toJson }}
      nodeSelector: {{ .NodeSelector | default dict | toJson }}
      containers:
        - name: helm
          image: {{.Image}}
          imagePullPolicy: {{ .ImagePullPolicy | default (hasSuffix .Image "-nightly" | ternary "Always" "IfNotPresent") }}
          command:
            - bash
            - "-c"
            - |+ #bash
              set -o nounset
              set -o pipefail
              set -o errexit

              {{ range $k, $v := .Pipeline }}
              with_prefix() {
                sed "s/^/\[{{$k}} {{$v.Chart.Name}}\] /"
              }

              echo ">>>>>>>> PIPELINE STEP (start) <<<<<<<<<<" | with_prefix


              {{- if .PreInstall }}
              cat > pre-install.sh <<EOF
              echo "running pre-install job script" 
              {{ .PreInstall | nindent 14 }}
              EOF

              bash pre-install.sh 2>&1 | with_prefix
              {{- end }}

              helm repo add helm-repo {{.ChartRepoURL}} 2>&1 | with_prefix
              helm repo update helm-repo 2>&1 | with_prefix

              cat > values.yml <<EOF
              {{ .HelmValuesYAML | nindent 14 }}
              EOF

              version_args=()
              if [ -n "{{.ChartVersion}}" ]; then
                version_args+=("--version" "{{.ChartVersion}}")
              fi

              helm upgrade --wait --install {{.ReleaseName}} helm-repo/{{.ChartName}} --namespace {{.ReleaseNamespace}} ${version_args[@]} --values values.yml 2>&1 | with_prefix | tee /dev/termination-log

              {{- if .PostInstall }}
              cat >post-install.sh <<EOF
              echo "running post-install job script"
              {{ .PostInstall | nindent 14 }}
              EOF

              bash post-install.sh 2>&1 | with_prefix
              {{- end }}

              echo ">>>>>>>> PIPELINE STEP (end) <<<<<<<<<<" | with_prefix
      restartPolicy: Never
  backoffLimit: {{ .BackOffLimit | default 1 | int}}
{{- end }}
