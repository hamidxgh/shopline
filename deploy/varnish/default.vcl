vcl 4.1;

backend default {
    .host = "nginx";
    .port = "80";
}

sub vcl_recv {
    if (req.method != "GET" && req.method != "HEAD") {
        return (pass);
    }
    if (req.url ~ "^/products/") {
        return (hash);
    }
    return (pass);
}

sub vcl_backend_response {
    if (bereq.url ~ "^/products/") {
        set beresp.ttl = 60s;
        unset beresp.http.Set-Cookie;
    }
}

sub vcl_deliver {
    if (obj.hits > 0) {
        set resp.http.X-Cache = "HIT";
    } else {
        set resp.http.X-Cache = "MISS";
    }
}
