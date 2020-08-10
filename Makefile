run:
	export `cat .env | xargs` && go run cmd/web.go

vet:
	go vet -mod=vendor ./...

test:
	# Run tests with cover profile.
	export `cat .env | xargs` && go test -mod=vendor ./...