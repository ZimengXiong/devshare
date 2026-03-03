FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /devshare ./cmd/devshare

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /devshare /usr/local/bin/devshare
VOLUME ["/var/lib/devshare"]
EXPOSE 8080
ENTRYPOINT ["devshare"]
CMD ["server"]
