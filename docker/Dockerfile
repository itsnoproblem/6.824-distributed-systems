FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/a-h/templ/cmd/templ generate && go build -o /out/tour ./cmd/tour

# The runtime keeps the Go toolchain: the eval runner executes `go test`
# inside the mounted lab repo.
FROM golang:1.25
WORKDIR /app
COPY --from=build /out/tour /app/tour
COPY content /app/content
# HOST=0.0.0.0: the app defaults to loopback, but the container must accept
# Docker's port-forwarded traffic; exposure is governed by the compose ports map.
ENV HOST=0.0.0.0 PORT=8080 DATA_DIR=/data CONTENT_DIR=/app/content LAB_REPO_DIR=/lab \
    GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomod
EXPOSE 8080
ENTRYPOINT ["/app/tour"]
