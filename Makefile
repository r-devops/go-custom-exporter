default:
	go mod init custom_exporter
	go get
	go build
	git add custom_exporter
	git commit -m Build
	git push
