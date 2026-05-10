# syntax=docker/dockerfile:1.7
# Build stage
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/aks-otel-zap .

# Runtime stage — distroless static, non-root
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aks-otel-zap /aks-otel-zap
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/aks-otel-zap"]
