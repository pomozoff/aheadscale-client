#include "CDescriptorHandoff.h"
#include <sys/socket.h>
#include <sys/un.h>
#include <errno.h>
#include <string.h>

static int check_socket(int fd, int expected_type) {
    int type = 0;
    socklen_t length = sizeof(type);
    if (getsockopt(fd, SOL_SOCKET, SO_TYPE, &type, &length) != 0) return -1;
    struct sockaddr_storage address;
    length = sizeof(address);
    if (getsockname(fd, (struct sockaddr *)&address, &length) != 0) return -1;
    if (type != expected_type || address.ss_family != AF_UNIX) {
        errno = EPROTOTYPE;
        return -1;
    }
    return 0;
}

int aheadscale_send_packet_fd(int control_fd, int packet_fd, const uint8_t generation[16]) {
    if (generation == NULL) { errno = EINVAL; return -1; }
    if (check_socket(control_fd, SOCK_DGRAM) != 0 || check_socket(packet_fd, SOCK_STREAM) != 0) return -1;
    int enabled = 1;
    if (setsockopt(control_fd, SOL_SOCKET, SO_NOSIGPIPE, &enabled, sizeof(enabled)) != 0) return -1;
    uint8_t payload[20] = {'A', 'S', 'D', '1'};
    memcpy(payload + 4, generation, 16);
    union { struct cmsghdr alignment; unsigned char bytes[CMSG_SPACE(sizeof(int))]; } control;
    memset(&control, 0, sizeof(control));
    struct iovec vector = { .iov_base = payload, .iov_len = sizeof(payload) };
    struct msghdr message;
    memset(&message, 0, sizeof(message));
    message.msg_iov = &vector;
    message.msg_iovlen = 1;
    message.msg_control = control.bytes;
    message.msg_controllen = sizeof(control.bytes);
    struct cmsghdr *rights = CMSG_FIRSTHDR(&message);
    rights->cmsg_level = SOL_SOCKET;
    rights->cmsg_type = SCM_RIGHTS;
    rights->cmsg_len = CMSG_LEN(sizeof(int));
    memcpy(CMSG_DATA(rights), &packet_fd, sizeof(packet_fd));
    // Never block an XPC callback on a full queue. Its caller owns retry/failure.
    ssize_t sent = sendmsg(control_fd, &message, MSG_DONTWAIT);
    if (sent == (ssize_t)sizeof(payload)) return 0;
    if (sent >= 0) errno = EIO;
    return -1;
}
