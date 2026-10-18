// Unit and HTTP parser checks: no launchd registration or upstream requests.
#define main font_service_entrypoint
#include "service.m"
#undef main
#include <assert.h>

static NSString *exchange(const char *request) {
    int pair[2];
    assert(socketpair(AF_UNIX, SOCK_STREAM, 0, pair) == 0);
    assert(send(pair[0], request, strlen(request), 0) == (ssize_t)strlen(request));
    handle(pair[1]);
    shutdown(pair[1], SHUT_WR);
    NSMutableData *bytes = [NSMutableData data];
    char part[2048];
    ssize_t count;
    while ((count = recv(pair[0], part, sizeof(part), 0)) > 0) [bytes appendBytes:part length:(NSUInteger)count];
    close(pair[0]); close(pair[1]);
    return [[NSString alloc] initWithData:bytes encoding:NSUTF8StringEncoding];
}
int main(void) {
    @autoreleasepool {
        allowed_files = [NSSet set];
        assert(valid_path(@"/Library/Fonts/한글 Font.otf"));
        assert(valid_path(@"/System/Library/Fonts/Collection.TTC"));
        assert(valid_path(@"/Library/Fonts/Collection.otc"));
        assert(valid_path(@"C:\\Fonts\\Test.TTF"));
        for (NSString *path in @[@"relative.ttf", @"/Fonts/../secret.ttf", @"/Fonts/..\\secret.ttf", @"/Fonts/a.ttf\n", @"/a.woff", @"https://a/test.ttf"]) assert(!valid_path(path));
        NSDictionary *catalog = @{@"fontFiles": @{@"/Library/Fonts/A.ttf": @[@{@"family": @"A"}], @"/System/Library/Fonts/Collection.ttc": @[@{@"family": @"Collection"}], @"/Library/Fonts/Collection.otc": @[@{@"family": @"Collection"}], @"relative.ttf": @[]}};
        assert(accept_catalog([NSJSONSerialization dataWithJSONObject:catalog options:0 error:NULL]));
        assert(allowed_files.count == 3 && [allowed_files containsObject:@"/Library/Fonts/A.ttf"]);
        assert([allowed_files containsObject:@"/System/Library/Fonts/Collection.ttc"]);
        assert([allowed_files containsObject:@"/Library/Fonts/Collection.otc"]);
        assert(!accept_catalog([@"{\"fontFiles\":{\"/A.ttf\":[{}]}}" dataUsingEncoding:NSUTF8StringEncoding]));
        BoundedBody body = {.bytes=NULL, .length=0, .limit=3, .exceeded=false};
        assert(receive_body("abc", 1, 3, &body) == 3);
        assert(receive_body("d", 1, 1, &body) == 0 && body.exceeded && body.length == 3);
        free(body.bytes);
        NSString *reply = exchange("GET /health HTTP/1.1\r\nHost: localhost:38474\r\nOrigin: null\r\n\r\n");
        assert([reply hasPrefix:@"HTTP/1.1 200"] && [reply containsString:@"figma-local-font-service"]);
        assert([exchange("OPTIONS /figma/font-files?freetype_minimum_api_version=20&isolate=true& HTTP/1.1\r\nHost: localhost:38474\r\nOrigin: null\r\nAccess-Control-Request-Method: GET\r\n\r\n") hasPrefix:@"HTTP/1.1 204"]);
        for (const char **request = (const char *[]){
            "GET /health HTTP/1.1\r\nHost: 127.0.0.1:38474\r\n\r\n",
            "GET /health HTTP/1.1\r\nHost: localhost:38474\r\nOrigin: https://evil.example\r\n\r\n",
            "GET /figma/font-file?file=%2FLibrary%2FFonts%2Fsecret.ttf HTTP/1.1\r\nHost: localhost:38474\r\n\r\n",
            "GET /figma/font-file?file=%2F..%2Fsecret.ttf HTTP/1.1\r\nHost: localhost:38474\r\n\r\n",
            "OPTIONS /health HTTP/1.1\r\nHost: localhost:38474\r\nAccess-Control-Request-Method: GET\r\nAccess-Control-Request-Headers: authorization\r\n\r\n", NULL}; *request; request++) {
            assert([exchange(*request) hasPrefix:@"HTTP/1.1 403"]);
        }
        assert([exchange("GET /health HTTP/1.1\r\nHost: localhost:38474\r\nHost: localhost:38474\r\n\r\n") hasPrefix:@"HTTP/1.1 400"]);
        assert([exchange("GET /health HTTP/1.1\r\nHost: localhost:38474\r\nTransfer-Encoding: chunked\r\n\r\n") hasPrefix:@"HTTP/1.1 400"]);
        puts("Native font service path, catalog, body bound and HTTP checks passed.");
    }
}
