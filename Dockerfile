FROM ubuntu:18.04 AS build

RUN apt-get update
RUN apt-get install -y wget git gcc curl

RUN wget -P /tmp https://dl.google.com/go/go1.16.7.linux-amd64.tar.gz

RUN tar -C /usr/local -xzf /tmp/go1.16.7.linux-amd64.tar.gz
RUN rm /tmp/go1.16.7.linux-amd64.tar.gz

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

WORKDIR /go/src/nfs-check
COPY . .
RUN go build -o /nfs-check .

FROM ubuntu:18.04
RUN apt update -y && \
    apt install -y duc 

COPY --from=build /nfs-check /nfs-check