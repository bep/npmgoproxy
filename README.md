This highly experimental and very much work in progress.

But as a concept, it actually works:

If you first start the proxy server with `go run main.go` you can do this from a Hugo project:

```
GOPROXY=http://localhost:8072 hugo mod get gohugo.io/npmjs/simple-icons/v5
```

The above will fetch the last version in the `v5` series from `npmjs.org`, verify the `shasum` and package it as a Go Module. There are still some missing pieces. For one, it does not follow dependencies.
