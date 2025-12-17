
prod:
	@echo "Building Windows amd64 executable..."
	@GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" \
	-o srv1c.exe .

prod32:
	@echo "Building Windows x386 executable..."
	@GOOS=windows GOARCH=386 go build -ldflags "-s -w" \
	-o srv1c.exe .

clean:
	@echo "Cleaning executables..."
	@rm -f srv1c.exe

%:
	@:




