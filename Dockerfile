FROM golang:1.26.5-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/torment ./cmd/torment

# tigerbeetle-go links a native library through cgo, so the runtime image needs the same glibc as the build.
FROM debian:trixie-slim
COPY --from=build /out/torment /usr/local/bin/torment
EXPOSE 8080
ENTRYPOINT ["torment"]
