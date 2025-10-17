# ---- Proto generation stage ----
FROM bufbuild/buf:1.44.0 AS proto
WORKDIR /src

# Copy only the files needed for code generation
COPY buf.yaml buf.gen.yaml buf.lock ./
COPY api/proto ./api/proto

# Generate gRPC + REST + OpenAPI code into /src/api/gen
RUN buf generate

# ---- Go build stage ----
FROM golang:1.23 AS build
ENV GOTOOLCHAIN=auto
ENV GO111MODULE=on
WORKDIR /app

# Copy Go module files first (for better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project source
COPY . .

# Copy generated code from proto stage
COPY --from=proto /src/api/gen ./api/gen

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o /out/api ./cmd/sentinel-api

# ---- Runtime stage ----
FROM gcr.io/distroless/base-debian12
WORKDIR /

# Copy compiled binary from build stage
COPY --from=build /out/api /api

# Expose ports: REST (8080), gRPC (8081), Prometheus metrics (9102)
EXPOSE 8080 8081 9102

ENTRYPOINT ["/api"]
