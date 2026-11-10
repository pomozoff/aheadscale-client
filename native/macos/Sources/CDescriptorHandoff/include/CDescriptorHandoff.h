#ifndef AHEADSCALE_DESCRIPTOR_HANDOFF_H
#define AHEADSCALE_DESCRIPTOR_HANDOFF_H
#include <stdint.h>
// Borrowed descriptors; sends exactly one stream FD over an inherited datagram.
int aheadscale_send_packet_fd(int control_fd, int packet_fd, const uint8_t generation[16]);
#endif
