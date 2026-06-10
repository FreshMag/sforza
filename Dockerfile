FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Pure-Go SQLite driver keeps CGO off, so the binary is fully static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sforza ./cmd/sforza

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sforza /usr/local/bin/sforza
ENV SFORZA_CONFIG=/etc/sforza/sforza.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sforza"]
