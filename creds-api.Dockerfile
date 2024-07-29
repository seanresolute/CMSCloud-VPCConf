FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM golang:1.14 as build
COPY . /build
RUN cd /build; CGO_ENABLED=0 GOBIN=/bin/ go install ./cmd/creds-api

FROM scratch
COPY --from=build /bin/creds-api /bin/creds-api
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bin/creds-api"]
