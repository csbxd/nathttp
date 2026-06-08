# nathttp

HTTP helpers for `github.com/csbxd/natnet`.

`nathttp` opens the listener with socket reuse before bind, starts a `natnet`
runner on the actual listener port, and ties both lifetimes to the supplied
context.

```go
err := nathttp.Serve(ctx, srv, nathttp.Config{
	NAT: natnet.Config{
		Syncer: cfClient,
	},
})
```

Use `ServeTLS` for certificate files or `ServeTLSConfig` for an existing
`tls.Config`.
