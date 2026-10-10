#include <errno.h>
#include <fcntl.h>
#include <linux/random.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <unistd.h>

/* Browser crypto supplies a fresh seed per VM. Emulated CPUs have no hardware RNG. */
int main(int argc, char **argv) {
    enum { seed_bytes = 32, hex_length = seed_bytes * 2 };
    struct {
        int entropy_count;
        int buffer_size;
        unsigned char seed[seed_bytes];
    } entropy = { .entropy_count = seed_bytes * 8, .buffer_size = seed_bytes };
    if (argc != 2 || strlen(argv[1]) != hex_length) return 1;
    for (int index = 0; index < seed_bytes; index++) {
        char digits[3] = { argv[1][index * 2], argv[1][index * 2 + 1], '\0' };
        if (!digits[0] || !digits[1]) return 1;
        char *end;
        long byte = strtol(digits, &end, 16);
        if (*end || byte < 0 || byte > 255) return 1;
        entropy.seed[index] = (unsigned char)byte;
    }
    if (argv[1][hex_length] != '\0') return 1;
    int descriptor = open("/dev/random", O_RDWR);
    if (descriptor < 0) return 1;
    int status = ioctl(descriptor, RNDADDENTROPY, &entropy);
    close(descriptor);
    if (status < 0) { perror("Initialize VM entropy"); return 1; }
    return 0;
}
