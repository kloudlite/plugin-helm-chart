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

              helm repo add helm-repo {{.ChartRepoURL}}
              helm repo update helm-repo

              {{- if .PreInstall }}
              echo "running pre-install job script"
              {{ .PreInstall | nindent 14 }}
              {{- end }}

              cat > values.yml <<EOF
              {{ .HelmValuesYAML | nindent 14 }}
              EOF

              version_args=()
              if [ -n "{{.ChartVersion}}" ]; then
                version_args+=("--version", "{{.ChartVersion}}")
              fi

              helm upgrade --install {{.ReleaseName}} helm-repo/{{.ChartName}} --namespace {{.ReleaseNamespace}} ${version_args[@]} --values values.yml 2>&1 | tee /dev/termination-log

              {{- if .PostInstall }}
              echo "running post-install job script"
              {{ .PostInstall | nindent 14 }}
              {{- end }}
            
      restartPolicy: Never
  backoffLimit: {{ .BackOffLimit | default 1 | int}}
{{- end }}
