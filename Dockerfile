FROM golang:1.22 AS build-stage
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /identity

# FROM build-stage AS test-stage
# RUN go test -v ./...

FROM gcr.io/distroless/base-debian11:99362bbf8d68fa5f878ba66bba92a8e969dbc38e AS release-stage
WORKDIR /
COPY --from=build-stage /identity /identity
# EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/identity"]
