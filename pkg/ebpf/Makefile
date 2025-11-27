.PHONY: generate clean

CLANG ?= clang
LLVM_STRIP ?= llvm-strip

generate: http_probe.o
	go run github.com/cilium/ebpf/cmd/bpf2go -type http_event httpProbe ./http_probe.o

http_probe.o: http_probe.c
	$(CLANG) -g -O2 -c -target bpf \
		-D__TARGET_ARCH_x86 \
		-I/usr/include/bpf \
		-o $@ $<
	$(LLVM_STRIP) -g $@

clean:
	rm -f *.o
	rm -f *_bpf*.go
	rm -f *_bpf*.o

