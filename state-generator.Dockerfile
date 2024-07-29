FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM golang:1.18 as build
COPY . /build
RUN cd /build; CGO_ENABLED=0 GOBIN=/bin/ go install ./cmd/state-generator

FROM scratch
COPY --from=build /bin/state-generator /bin/state-generator
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bin/state-generator"]
