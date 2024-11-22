{{- with .}}
{{- /* gotype: github.com/kloudlite/plugin-helm-chart/internal/controller/templates.UnInstallJobVars */ -}}
apiVersion: batch/v1
kind: Job
metadata:
  name:  {{.Metadata.Name | toJson }}
  namespace: {{ .Metadata.Namespace | toJson }}
  labels: {{.Metadata.Labels | toJson }}
  annotations: {{.Metadata.Annotations | toJson }}
  ownerReferences: {{ .Metadata.OwnerReferences | default list | toJson }}
spec:
  template:
    metadata:
      annotations: {{.ObservabilityAnnotations | toJson }}
    spec:
      serviceAccountName: {{.ServiceAccountName | toJson }}
      tolerations: {{.Tolerations | toJson }}
      affinity: {{.Affinity | toJson}}
      nodeSelector: {{ .NodeSelector | toJson }}
      containers:
        - name: helm
          image: {{.Image}}
          imagePullPolicy: {{ .ImagePullPolicy | default (hasSuffix .Image "-nightly" | ternary "Always" "IfNotPresent" ) }}
          command:
            - bash
            - -c
            - |+ #bash
              set -o nounset
              set -o pipefail
              set -o errexit

              helm repo add helm-repo {{.ChartRepoURL}}
              helm repo update helm-repo

              {{- if .PreUninstall }}
              echo "running pre-uninstall job script"
              {{ .PreUninstall | nindent 14 }}
              {{- end }}

              helm uninstall {{.ReleaseName}} --namespace {{.ReleaseNamespace}} | tee /dev/termination-log

              {{- if .PostUninstall }}
              echo "running post-uninstall job script"
              {{ .PostUninstall | nindent 14 }}
              {{- end }}
            
      restartPolicy: Never
  backoffLimit: {{ .BackOffLimit | default 1 | int}}
{{- end }}
