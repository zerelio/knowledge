FROM golang:1.26-alpine AS builder

# Build context is the repo root (same directory as Dockerfile).
# The release workflow checks out plugin-sdk at ./plugin-sdk and rewrites
# the go.mod replace directive to ./plugin-sdk before building, so both
# the COPY below and go mod download resolve correctly.

WORKDIR /src

# Copy the plugin-sdk replacement first so go mod download can resolve it.
COPY plugin-sdk/ /src/plugin-sdk/

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false \
    -ldflags="-s -w" \
    -o /bin/zerelio \
    ./cmd/zerelio

FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/zerelio /zerelio

EXPOSE 8080
ENTRYPOINT ["/zerelio"]
