FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM golang:1.18 as build
COPY . /build
RUN cd /build; CGO_ENABLED=0 GOBIN=/bin/ go install ./cmd/vpc-conf

FROM scratch
COPY --from=build /bin/vpc-conf /bin/vpc-conf
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bin/vpc-conf"]
