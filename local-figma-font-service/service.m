// macOS launchd socket-activated local Figma font relay.
// No direct font filesystem reads, child processes, shell execution, or standalone bind fallback.
#import <Foundation/Foundation.h>
#include <curl/curl.h>
#include <arpa/inet.h>
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <launch.h>
#include <poll.h>
#include <signal.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>
#include <uuid/uuid.h>

#define PORT 38474
#define HEADER_LIMIT 8192
#define IDLE_SECONDS 120
#define REQUEST_SECONDS 5

static volatile sig_atomic_t stopping = 0;
static char session_id[37];
static char started_at[40];
static double last_activity;

static void on_signal(int signal_number) { (void)signal_number; stopping = 1; }
static double monotonic_seconds(void) {
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return (double)now.tv_sec + (double)now.tv_nsec / 1000000000.0;
}

static bool wait_for(int fd, short events, double deadline) {
    while (!stopping) {
        double remaining = deadline - monotonic_seconds();
        if (remaining <= 0) return false;
        struct pollfd item = { .fd = fd, .events = events, .revents = 0 };
        int result = poll(&item, 1, (int)(remaining * 1000) + 1);
        if (result > 0) return (item.revents & events) != 0;
        if (result == 0 || errno != EINTR) return false;
    }
    return false;
}

static bool send_bytes(int fd, const void *bytes, size_t length, double deadline) {
    size_t sent = 0;
    while (sent < length && wait_for(fd, POLLOUT, deadline)) {
        ssize_t count = send(fd, (const char *)bytes + sent, length - sent, 0);
        if (count > 0) sent += (size_t)count;
        else if (count == 0 || (errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK)) return false;
    }
    return sent == length;
}
static void response_bytes(int fd, const char *status, const void *bytes, size_t length, const char *type, bool preflight) {
    char header[1024];
    int count = snprintf(header, sizeof(header),
        "HTTP/1.1 %s\r\nContent-Type: %s\r\nContent-Length: %zu\r\n"
        "Connection: close\r\nCache-Control: no-store\r\nX-Content-Type-Options: nosniff\r\n"
        "Access-Control-Allow-Origin: null\r\nVary: Origin\r\n%s\r\n", status, type, length,
        preflight ? "Access-Control-Allow-Methods: GET\r\nAccess-Control-Max-Age: 600\r\nAccess-Control-Allow-Private-Network: true\r\n" : "");
    double deadline = monotonic_seconds() + REQUEST_SECONDS;
    if (count > 0 && (size_t)count < sizeof(header) && send_bytes(fd, header, (size_t)count, deadline))
        send_bytes(fd, bytes, length, deadline);
}
static void respond(int fd, const char *status, const char *body, bool preflight) {
    response_bytes(fd, status, body, strlen(body), "application/json", preflight);
}

static NSSet<NSString *> *allowed_files;
static bool valid_path(id candidate) {
    if (![candidate isKindOfClass:[NSString class]]) return false;
    NSString *path = candidate;
    if (!path.length || path.length > 4096 || [path rangeOfString:@"://"].location != NSNotFound) return false;
    for (NSUInteger i = 0; i < path.length; i++) {
        unichar c = [path characterAtIndex:i];
        if (c < 32 || c == 127) return false;
    }
    bool absolute = [path hasPrefix:@"/"];
    if (!absolute && path.length >= 3) {
        unichar drive = [path characterAtIndex:0];
        absolute = ((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) &&
            [path characterAtIndex:1] == ':' && ([path characterAtIndex:2] == '/' || [path characterAtIndex:2] == '\\');
    }
    if (!absolute || [[path componentsSeparatedByCharactersInSet:[NSCharacterSet characterSetWithCharactersInString:@"/\\"]] containsObject:@".."]) return false;
    NSString *ext = path.pathExtension.lowercaseString;
    return [ext isEqualToString:@"ttf"] || [ext isEqualToString:@"otf"] ||
        [ext isEqualToString:@"ttc"] || [ext isEqualToString:@"otc"];
}
static bool accept_catalog(NSData *bytes) {
    id value = [NSJSONSerialization JSONObjectWithData:bytes options:0 error:NULL];
    if (![value isKindOfClass:[NSDictionary class]]) return false;
    id catalog = value[@"fontFiles"];
    if (![catalog isKindOfClass:[NSDictionary class]] || [catalog count] > 50000) return false;
    NSMutableSet *files = [NSMutableSet set];
    for (NSString *candidate in catalog) {
        id faces = catalog[candidate];
        if (![faces isKindOfClass:[NSArray class]] || [faces count] > 256) return false;
        for (id face in faces) {
            if (![face isKindOfClass:[NSDictionary class]] || ![face[@"family"] isKindOfClass:[NSString class]]) return false;
        }
        if (valid_path(candidate)) [files addObject:candidate];
    }
    allowed_files = [files copy];
    return true;
}
typedef struct { void *bytes; size_t length; size_t limit; bool exceeded; } BoundedBody;
static size_t receive_body(char *bytes, size_t size, size_t count, void *context) {
    BoundedBody *body = context;
    if (size && count > SIZE_MAX / size) { body->exceeded = true; return 0; }
    size_t length = size * count;
    if (length > body->limit - body->length) { body->exceeded = true; return 0; }
    if (!length) return 0;
    void *grown = realloc(body->bytes, body->length + length);
    if (!grown) return 0;
    body->bytes = grown;
    memcpy((char *)body->bytes + body->length, bytes, length);
    body->length += length;
    return length;
}
static void relay(int fd, bool list, NSString *file) {
    BoundedBody body = { .bytes = NULL, .length = 0, .limit = list ? 8000000 : 20000000, .exceeded = false };
    CURL *curl = curl_easy_init();
    const char *error_code = "UPSTREAM_FAILED";
    bool success = false;
    if (curl) {
        char *escaped = file ? curl_easy_escape(curl, file.UTF8String, 0) : NULL;
        NSString *url = list ? @"http://localhost:44950/figma/font-files?freetype_minimum_api_version=20&isolate=true&" :
            (escaped ? [@"http://localhost:44950/figma/font-file?file=" stringByAppendingString:[NSString stringWithUTF8String:escaped]] : nil);
        struct curl_slist *headers = curl_slist_append(NULL, "Origin: https://www.figma.com");
        if (url && headers) {
            curl_easy_setopt(curl, CURLOPT_URL, url.UTF8String);
            curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
            curl_easy_setopt(curl, CURLOPT_PROXY, "");
            curl_easy_setopt(curl, CURLOPT_NETRC, (long)CURL_NETRC_IGNORED);
            curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 0L);
            curl_easy_setopt(curl, CURLOPT_PROTOCOLS, (long)CURLPROTO_HTTP);
            curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, 12000L);
            curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT_MS, 3000L);
            curl_easy_setopt(curl, CURLOPT_NOSIGNAL, 1L);
            curl_easy_setopt(curl, CURLOPT_MAXFILESIZE_LARGE, (curl_off_t)body.limit);
            curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, receive_body);
            curl_easy_setopt(curl, CURLOPT_WRITEDATA, &body);
            CURLcode result = curl_easy_perform(curl);
            long status = 0;
            curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &status);
            if (result == CURLE_OPERATION_TIMEDOUT) error_code = "UPSTREAM_TIMEOUT";
            if (body.exceeded || result == CURLE_FILESIZE_EXCEEDED) error_code = "SIZE_LIMIT";
            success = result == CURLE_OK && status >= 200 && status < 300;
            if (success && list) success = accept_catalog([NSData dataWithBytes:body.bytes length:body.length]);
        }
        curl_slist_free_all(headers);
        curl_free(escaped);
        curl_easy_cleanup(curl);
    }
    if (success) response_bytes(fd, "200 OK", body.bytes, body.length, list ? "application/json" : "application/octet-stream", false);
    else {
        if (list) allowed_files = [NSSet set];
        char error[80];
        snprintf(error, sizeof(error), "{\"error\":\"%s\"}", error_code);
        respond(fd, "502 Bad Gateway", error, false);
        fprintf(stderr, "font-service: %s\n", error_code);
    }
    free(body.bytes);
}

static char *trim(char *value) {
    while (*value == ' ' || *value == '\t') value++;
    size_t length = strlen(value);
    while (length && (value[length - 1] == ' ' || value[length - 1] == '\t')) value[--length] = '\0';
    return value;
}

static void handle(int fd) {
    char buffer[HEADER_LIMIT + 1];
    size_t length = 0;
    char *header_end = NULL;
    double deadline = monotonic_seconds() + REQUEST_SECONDS;
    while (length < HEADER_LIMIT && wait_for(fd, POLLIN, deadline)) {
        ssize_t count = recv(fd, buffer + length, HEADER_LIMIT - length, 0);
        if (count < 0 && (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK)) continue;
        if (count <= 0) return;
        // Reject embedded NULs so string parsing cannot ignore hidden headers.
        if (memchr(buffer + length, '\0', (size_t)count)) { respond(fd, "400 Bad Request", "", false); return; }
        length += (size_t)count;
        buffer[length] = '\0';
        header_end = strstr(buffer, "\r\n\r\n");
        if (header_end) break;
    }
    if (!header_end) {
        respond(fd, length == HEADER_LIMIT ? "431 Request Header Fields Too Large" : "408 Request Timeout", "", false);
        return;
    }
    *header_end = '\0';
    char *line_end = strstr(buffer, "\r\n");
    if (!line_end) { respond(fd, "400 Bad Request", "", false); return; }
    *line_end = '\0';
    char method[16], target[8193], version[16], extra;
    if (sscanf(buffer, "%15s %8192s %15s %c", method, target, version, &extra) != 3 || strcmp(version, "HTTP/1.1")) {
        respond(fd, "400 Bad Request", "", false); return;
    }
    char *host = NULL, *origin = NULL, *request_method = NULL, *request_headers = NULL;
    bool content_length_seen = false;
    char *line = line_end + 2;
    while (*line) {
        char *next = strstr(line, "\r\n");
        if (next) *next = '\0';
        char *colon = strchr(line, ':');
        if (!colon || line[0] == ' ' || line[0] == '\t') { respond(fd, "400 Bad Request", "", false); return; }
        *colon = '\0';
        char *value = trim(colon + 1);
        char **destination = NULL;
        if (!strcasecmp(line, "Host")) destination = &host;
        else if (!strcasecmp(line, "Origin")) destination = &origin;
        else if (!strcasecmp(line, "Access-Control-Request-Method")) destination = &request_method;
        else if (!strcasecmp(line, "Access-Control-Request-Headers")) destination = &request_headers;
        else if (!strcasecmp(line, "Transfer-Encoding")) { respond(fd, "400 Bad Request", "", false); return; }
        else if (!strcasecmp(line, "Content-Length")) {
            if (content_length_seen || strcmp(value, "0")) { respond(fd, "400 Bad Request", "", false); return; }
            content_length_seen = true;
        }
        if (destination) {
            if (*destination) { respond(fd, "400 Bad Request", "", false); return; }
            *destination = value;
        }
        if (!next) break;
        line = next + 2;
    }
    if (!host || strcmp(host, "localhost:38474") ||
        (origin && strcmp(origin, "null"))) { respond(fd, "403 Forbidden", "", false); return; }
    NSString *request_target = [NSString stringWithUTF8String:target];
    bool list = [request_target isEqualToString:@"/figma/font-files?freetype_minimum_api_version=20&isolate=true&"];
    bool health = [request_target isEqualToString:@"/health"];
    bool is_file = [request_target hasPrefix:@"/figma/font-file?file="] && [request_target rangeOfString:@"&"].location == NSNotFound &&
        [request_target rangeOfString:@"#"].location == NSNotFound;
    NSString *file = is_file ? [[[request_target substringFromIndex:22] stringByReplacingOccurrencesOfString:@"+" withString:@" "] stringByRemovingPercentEncoding] : nil;
    if (!list && !is_file && !health) { respond(fd, "404 Not Found", "", false); return; }
    if (!strcmp(method, "OPTIONS")) {
        if (!request_method || strcmp(request_method, "GET") || (request_headers && *request_headers)) {
            respond(fd, "403 Forbidden", "", false); return;
        }
        respond(fd, "204 No Content", "", true);
        return;
    }
    if (strcmp(method, "GET")) { respond(fd, "405 Method Not Allowed", "", false); return; }
    last_activity = monotonic_seconds();
    if (is_file && (!valid_path(file) || ![allowed_files containsObject:file])) { respond(fd, "403 Forbidden", "", false); return; }
    if (!health) { relay(fd, list, file); last_activity = monotonic_seconds(); return; }
    char body[512];
    snprintf(body, sizeof(body),
        "{\"service\":\"figma-local-font-service\",\"version\":1,\"activationKind\":\"launchd-socket\","
        "\"pid\":%ld,\"sessionId\":\"%s\",\"startedAt\":\"%s\",\"idleExitSeconds\":120,\"fontRelay\":true}",
        (long)getpid(), session_id, started_at);
    respond(fd, "200 OK", body, false);
}

int main(void) {
    @autoreleasepool {
    if (curl_global_init(CURL_GLOBAL_DEFAULT) != CURLE_OK) return 1;
    allowed_files = [NSSet set];
    signal(SIGPIPE, SIG_IGN);
    signal(SIGTERM, on_signal);
    signal(SIGINT, on_signal);
    int *fds = NULL;
    size_t count = 0;
    int result = launch_activate_socket("FontHTTP", &fds, &count);
    if (result) { fprintf(stderr, "launch_activate_socket: %s\n", strerror(result)); return 1; }
    if (!count || count > 4) { fprintf(stderr, "Unexpected inherited socket count\n"); free(fds); return 1; }
    struct pollfd listeners[4];
    for (size_t i = 0; i < count; i++) {
        struct sockaddr_in address;
        socklen_t address_size = sizeof(address);
        memset(&address, 0, sizeof(address));
        if (getsockname(fds[i], (struct sockaddr *)&address, &address_size) || address.sin_family != AF_INET ||
            ntohl(address.sin_addr.s_addr) != INADDR_LOOPBACK || ntohs(address.sin_port) != PORT) {
            fprintf(stderr, "Inherited socket is not the expected IPv4 loopback endpoint\n"); free(fds); return 1;
        }
        int socket_type = 0, listening = 0;
        socklen_t type_size = sizeof(socket_type), listen_size = sizeof(listening);
        int type_result = getsockopt(fds[i], SOL_SOCKET, SO_TYPE, &socket_type, &type_size);
        int type_error = type_result ? errno : 0;
        int listen_result = getsockopt(fds[i], SOL_SOCKET, SO_ACCEPTCONN, &listening, &listen_size);
        int listen_error = listen_result ? errno : 0;
        // Darwin may define SO_ACCEPTCONN but reject querying it with ENOPROTOOPT,
        // including ordinary socket()+bind()+listen() sockets on current macOS.
        // Keep launchd inheritance, exact loopback endpoint and SO_TYPE mandatory;
        // require a listening value whenever this optional query is supported.
        bool listening_verified = listen_result == 0 ? listening != 0 : listen_error == ENOPROTOOPT;
        if (type_result || socket_type != SOCK_STREAM || !listening_verified) {
            fprintf(stderr, "Inherited socket validation: typeResult=%d typeErrno=%d type=%d listenResult=%d listenErrno=%d listening=%d\n",
                    type_result, type_error, socket_type, listen_result, listen_error, listening);
            free(fds); return 1;
        }
        int flags = fcntl(fds[i], F_GETFL);
        if (flags < 0 || fcntl(fds[i], F_SETFL, flags | O_NONBLOCK) < 0) { free(fds); return 1; }
        listeners[i] = (struct pollfd){ .fd = fds[i], .events = POLLIN, .revents = 0 };
    }
    uuid_t identifier;
    uuid_generate_random(identifier);
    uuid_unparse_lower(identifier, session_id);
    struct timespec now;
    clock_gettime(CLOCK_REALTIME, &now);
    struct tm utc;
    gmtime_r(&now.tv_sec, &utc);
    char date[24];
    strftime(date, sizeof(date), "%Y-%m-%dT%H:%M:%S", &utc);
    snprintf(started_at, sizeof(started_at), "%s.%03ldZ", date, now.tv_nsec / 1000000);
    last_activity = monotonic_seconds();
    while (!stopping) {
        double remaining = IDLE_SECONDS - (monotonic_seconds() - last_activity);
        if (remaining <= 0) break;
        int ready = poll(listeners, (nfds_t)count, (int)(remaining * 1000) + 1);
        if (ready < 0) { if (errno == EINTR) continue; break; }
        if (!ready) break;
        for (size_t i = 0; i < count && !stopping; i++) {
            if (!(listeners[i].revents & POLLIN)) continue;
            struct sockaddr_in peer;
            socklen_t peer_size = sizeof(peer);
            int client = accept(fds[i], (struct sockaddr *)&peer, &peer_size);
            if (client < 0) continue;
            int flags = fcntl(client, F_GETFL);
            if (peer.sin_family == AF_INET && ntohl(peer.sin_addr.s_addr) == INADDR_LOOPBACK &&
                flags >= 0 && fcntl(client, F_SETFL, flags | O_NONBLOCK) == 0) { @autoreleasepool { handle(client); } }
            close(client);
        }
    }
    for (size_t i = 0; i < count; i++) close(fds[i]);
    free(fds);
    curl_global_cleanup();
    return 0;
    }
}
