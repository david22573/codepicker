BINARY_NAME=codepicker

all: build

build:
	go build -o $(BINARY_NAME) main.go

install:
	go install

clean:
	go clean
	rm -f $(BINARY_NAME) codepicker_context.txt
	rm -rf codepicker_out
