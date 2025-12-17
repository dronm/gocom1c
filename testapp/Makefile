
prod:
	@echo "Building Windows amd64 executable..."
	@GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" \
	-o comtest64.exe .

prod32:
	@echo "Building Windows x386 executable..."
	@GOOS=windows GOARCH=386 go build -ldflags "-s -w" \
	-o comtest32.exe .

clear:
	@echo "Cleaning executables..."
	@rm -f comtest64.exe comtest32.exe

%:
	@:




