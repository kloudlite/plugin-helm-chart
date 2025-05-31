{{- with .}}
template:
  metadata:
    annotations: {{.PodAnnotations | default dict | toJson }}
  spec:
    serviceAccountName: {{.ServiceAccountName | toJson }}
    tolerations: {{ .PodTolerations | default list | toJson }}
    nodeSelector: {{ .NodeSelector | default dict | toJson }}
    securityContext:
      runAsNonRoot: true
      runAsUser: 65532 # nonroot user from gcr.io/distroless/static:nonroot image
      runAsGroup: 65532 # nonroot group from gcr.io/distroless/static:nonroot image
      allowPrivilegeEscalation: false
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
            {{ with $v }}
            with_prefix() {
              sed "s/^/\[{{$k}} {{.Chart.Name}}\] /"
            }

            echo ">>>>>>>> PIPELINE STEP start <<<<<<<<<<" | with_prefix

            {{- if .PreUninstall }}
            cat > pre-uninstall.sh <<'END_PRE_UNINSTALL'
            echo "running pre-uninstall job script" 
            {{ .PreUninstall | nindent 14 }}
            END_PRE_UNINSTALL

            bash pre-uninstall.sh 2>&1 | with_prefix
            {{- end }}

            set +o pipefail
            helm uninstall {{.Release.Name}} --namespace {{.Release.Namespace}} 2>&1 | with_prefix | tee /dev/termination-log
            set -o pipefail

            {{- if .PostUninstall }}
            cat >post-uninstall.sh <<'END_POST_INSTALL'
            echo "running post-uninstall job script"
            {{ .PostUninstall | nindent 14 }}
            END_POST_INSTALL

            bash post-uninstall.sh 2>&1 | with_prefix
            {{- end }}

            {{ end }}

            echo ">>>>>>>> PIPELINE STEP end <<<<<<<<<<" | with_prefix
            {{ end }}
    restartPolicy: Never
backoffLimit: {{ .BackOffLimit | default 1}}
{{- end }}
