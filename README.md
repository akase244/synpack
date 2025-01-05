# Synpack - A Lightweight TCP Heartbeat Tool
Synpack(/ʃinpaku/) is a lightweight network tool for testing and troubleshooting network connectivity by sending TCP SYN packets.
Its name is derived from the Japanese word "心拍" (shinpaku), meaning "heartbeat".

## Usage

```
$ go build -o synpack
$ sudo ./synpack -host github.com -port 80 -c 3
Synpack eth0 (192.168.0.1) -> github.com (20.27.177.113)
len=4096 ip=20.27.177.113 port=80 seq=3884279077 rtt=81.550847ms
len=4096 ip=20.27.177.113 port=80 seq=2575736597 rtt=81.027256ms
len=4096 ip=20.27.177.113 port=80 seq=3729930979 rtt=77.739979ms

--- github.com Synpack statistic ---
3 packets transmitted, 3 packets received, 0.00% packet loss
round-trip min/avg/max = 77.739979ms/81.550847ms/80.106027ms
```