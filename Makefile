BINARY_NAME=codepicker

all: build

build:
	go build -o $(BINARY_NAME) main.go

install:
	go install

clean:
	go clean
	rm -f $(BINARY_NAME)
	# Removes default txt, and any generated _context.md files
	rm -f codepicker_context.txt *_context.md
	rm -rf codepicker_out

