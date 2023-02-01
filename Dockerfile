FROM alpine:3.17

RUN mkdir -p /mnt/src \
    && mkdir -p /mnt/dst
RUN apk --no-cache add curl \
    && curl -Ls https://dl.k8s.io/release/v1.23.5/bin/linux/amd64/kubectl --output /usr/bin/kubectl \
    && chmod +x /usr/bin/kubectl

COPY p2pcp /usr/bin 
COPY script/entrypoint.sh /usr/bin

EXPOSE 10090
ENTRYPOINT ["/usr/bin/entrypoint.sh"] 
