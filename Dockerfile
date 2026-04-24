# Copied from https://docs.docker.com/language/golang/build-images/

FROM golang:1.23 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd      ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /farm ./cmd/farm

# Deploy the application binary into a lean image
FROM cgr.dev/chainguard/static:latest AS build-release-stage

WORKDIR /

COPY --from=build-stage /farm /farm

USER nonroot:nonroot

ENTRYPOINT ["/farm"]
