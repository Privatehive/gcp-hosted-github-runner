FROM --platform=$BUILDPLATFORM golang:1.24 AS build

WORKDIR /build
COPY ./go.mod ./
COPY ./go.sum ./
COPY ./pkg ./pkg
COPY main.go ./
RUN go mod download

ARG UID=11010 GID=11010
RUN echo "scaler:*:$UID:$GID::/:" > passwd && echo "scaler:*:$GID:" > group

ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -a -tags netgo,osusergo -trimpath -ldflags '-w -s -buildid= -extldflags "-static"' -o autoscaler main.go


FROM scratch AS run
LABEL org.opencontainers.image.title="github-runner-autoscaler"
LABEL org.opencontainers.image.authors="Björn Stresing <bjoern.stresing@gmx.de>"
LABEL org.opencontainers.image.description="This image provides a webserver that listens for GitHub actions webhooks and provision compute instances if needed"
LABEL org.opencontainers.image.source="https://github.com/Privatehive/gcp-hosted-github-runner"
LABEL org.opencontainers.image.url="https://github.com/users/Privatehive/packages/container/package/runner-autoscaler"

WORKDIR /rundir
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /build/passwd /build/group /etc/
COPY --from=build /build/autoscaler ./
ENV GIN_MODE=release
USER scaler:scaler
EXPOSE 8080/tcp
ENTRYPOINT ["/rundir/autoscaler"]
