#include <stdio.h>
#define MAX(a, b) ((a) > (b) ? (a) : (b))

/* Block comment */
static const char *name = "util";
typedef struct { int x; float y; } point_t;

int sum(const int *xs, size_t n) {
    int total = 0;
    for (size_t i = 0; i < n; ++i) total += xs[i] << 1;
    return total ? total : -1; // fallback
}
