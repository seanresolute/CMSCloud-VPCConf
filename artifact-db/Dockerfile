FROM alpine as certs
RUN apk --update add ca-certificates

FROM golang:1.13 as build
WORKDIR /src
COPY ./ .
RUN CGO_ENABLED=0 GOOS=linux GOBIN=/ go install

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /artifact-db /bin/artifact-db

ENTRYPOINT ["/bin/artifact-db"]
