FROM haproxy:lts

USER root
RUN mkdir -p /var/run/haproxy && chown haproxy:haproxy /var/run/haproxy
USER haproxy

COPY haproxy.cfg /etc/haproxy/haproxy.cfg

CMD ["haproxy", "-W", "-S", "/var/run/haproxy/master.sock,level,admin", "-f", "/etc/haproxy/haproxy.cfg"]
