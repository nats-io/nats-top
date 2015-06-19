# nats-top

Top like program monitor for gnatsd written in Go.

```sh
go get github.com/nats-io/nats-top

Usage: nats-top [-s server] [-m monitor] [-n num_connections] [-d delay_secs]
```

Example Output:

```sh
Server:
  Load: CPU: 0.0% Memory: 5.5M
  In:   Msgs: 2.2K  Bytes: 8.0K  Msgs/Sec: 1.0  Bytes/Sec: 1.0
  Out:  Msgs: 2.2K  Bytes: 8.0K  Msgs/Sec: 1.0  Bytes/Sec: 1.0

Connections: 1
  HOST                 CID      SUBS    PENDING     MSGS_TO     MSGS_FROM   BYTES_TO    BYTES_FROM
  127.0.0.1:56358      4        1       0           1.2K        1.2K        4.5K        4.5K
```
