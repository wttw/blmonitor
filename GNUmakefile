.PHONY: build
build:
	go build -C cmd/blmonitor
	go build -C cmd/spamcop-inject

.PHONY: resetdb
resetdb:
	psql -f schema/down-01-00-initial.sql blmonitor
	psql -f schema/patch-00-01-initial.sql blmonitor
	psql -f schema/populate.sql blmonitor

.PHONY: deploy
deploy:
	GOOS=linux GOARCH=amd64 go build -C cmd/blmonitor
	GOOS=linux GOARCH=amd64 go build -C cmd/spamcop-inject
	scp cmd/blmonitor/blmonitor cmd/spamcop-inject/spamcop-inject schema/* blmonitor.service deploy.sh mx:blmonitor
