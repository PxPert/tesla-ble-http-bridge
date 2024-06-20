FROM --platform=$BUILDPLATFORM golang:1.22.2 as build

WORKDIR /app

# Download Go modules
COPY src/go.mod src/go.sum src/*.go ./
RUN go mod download

# Build
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -o /tesla-ble-http-bridge

FROM alpine:3.14 as final
COPY --from=build /tesla-ble-http-bridge /tesla-ble-http-bridge


EXPOSE 3333
# Run
CMD ["/tesla-ble-http-bridge"]
