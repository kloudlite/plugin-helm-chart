{{- with .}}
template:
  metadata:
    annotations: {{.PodAnnotations | default dict | toJson }}
  spec:
    serviceAccountName: {{.ServiceAccountName | toJson }}
    tolerations: {{ .PodTolerations | default list | toJson }}
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
            {{ with $v }}
            with_prefix() {
              sed "s/^/\[{{$k}}| {{.Chart.Name}}\] /"
            }

            echo ">>>>>>>> PIPELINE STEP start <<<<<<<<<<" | with_prefix

            {{- if .PreInstall }}
            cat > pre-install.sh <<'END_PRE_INSTALL'
            echo "running pre-install job script" 
            {{ .PreInstall | nindent 12 }}
            END_PRE_INSTALL

            bash pre-install.sh 2>&1 | with_prefix
            {{- end }}

            helm repo add helm-repo-{{$k}} {{.Chart.URL}} 2>&1 | with_prefix
            helm repo update helm-repo-{{$k}} 2>&1 | with_prefix

            cat > values.yml <<'EOF'
            {{ .HelmValuesYAML | nindent 12 }}
            EOF

            version_args=()
            if [ -n "{{.Chart.Version}}" ]; then
              version_args+=("--version" "{{.Chart.Version}}")
            fi

            set +o pipefail
            helm upgrade --debug --wait --install {{.Release.Name}} helm-repo-{{$k}}/{{.Chart.Name}} --namespace {{.Release.Namespace}} ${version_args[@]} --values values.yml 2>&1 | sed -n '/USER-SUPPLIED VALUES:/q;p' | with_prefix | tee /dev/termination-log
            set -o pipefail

            {{- if .PostInstall }}
            cat >post-install.sh <<'END_POST_INSTALL'
            echo "running post-install job script"
            {{ .PostInstall | nindent 12 }}
            END_POST_INSTALL

            bash post-install.sh 2>&1 | with_prefix
            {{- end }}

            {{ end }}

            echo ">>>>>>>> PIPELINE STEP end <<<<<<<<<<" | with_prefix
            {{ end}}
    restartPolicy: Never
backoffLimit: {{ .BackOffLimit | default 1}}
{{- end }}
