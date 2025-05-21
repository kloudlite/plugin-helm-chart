FROM gcr.io/distroless/static:nonroot
WORKDIR /
ARG BINARY TARGETARCH
COPY $BINARY-$TARGETARCH /plugin-helm-controller
USER 65532:65532
ENTRYPOINT ["/plugin-helm-controller"]
