# Synpack - A Lightweight TCP Heartbeat Tool
Synpack(/ʃinpaku/) is a lightweight network tool for testing and troubleshooting network connectivity by sending TCP SYN packets.
Its name is derived from the Japanese word "心拍" (shinpaku), meaning "heartbeat".

## Usage

```
$ CGO_ENABLED=0 go build -o synpack
$ sudo ./synpack -h github.com -p 80 -c 3
Synpack eth0 (192.168.0.1) -> github.com (20.27.177.113)
len=4096 ip=20.27.177.113 port=80 seq=344846291 rtt=29.27 ms attempt=0 times
len=4096 ip=20.27.177.113 port=80 seq=2709490451 rtt=25.81 ms attempt=0 times
len=4096 ip=20.27.177.113 port=80 seq=2965481930 rtt=28.46 ms attempt=0 times

--- github.com Synpack statistic ---
3 packets transmitted, 3 packets received, 0.00% packet loss
round-trip min/avg/max = 25.81/27.85/29.27 ms
```

## Supported Environment

- Ubuntu 22.04
