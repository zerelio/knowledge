FROM golang:1.26-alpine AS builder

# Build context is the workspace root (parent of zerelio/ and plugin-sdk/).
# go.mod uses: replace github.com/zerelio/plugin-sdk => ../plugin-sdk
# so both modules must be present under /workspace/.

WORKDIR /workspace

# Copy both modules into the workspace so the replace directive resolves.
COPY plugin-sdk/ ./plugin-sdk/
COPY zerelio/go.mod zerelio/go.sum ./zerelio/

WORKDIR /workspace/zerelio
RUN go mod download

COPY zerelio/ .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false \
    -ldflags="-s -w" \
    -o /bin/zerelio \
    ./cmd/zerelio

FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/zerelio /zerelio

EXPOSE 8080
ENTRYPOINT ["/zerelio"]
