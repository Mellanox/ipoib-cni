FROM golang:1.24-alpine as builder

COPY . /usr/src/ipoib-cni

ENV HTTP_PROXY $http_proxy
ENV HTTPS_PROXY $https_proxy

RUN apk add --no-cache --virtual build-dependencies build-base linux-headers
WORKDIR /usr/src/ipoib-cni
RUN make clean && \
    make build

FROM alpine:3
COPY --from=builder /usr/src/ipoib-cni/build/ipoib /usr/bin/
WORKDIR /

LABEL io.k8s.display-name="IPoIB CNI"

COPY ./images/entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
