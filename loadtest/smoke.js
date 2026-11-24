// k6 load test (https://k6.io, single binary):  k6 run loadtest/smoke.js
// BASE_URL=https://staging.example.com k6 run loadtest/smoke.js
// Starts gentle; raise VUS to find the breaking point of your VPS BEFORE launch.
import http from "k6/http";
import { check, sleep } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8080";

export const options = {
  stages: [
    { duration: "30s", target: Number(__ENV.VUS || 50) },
    { duration: "1m", target: Number(__ENV.VUS || 50) },
    { duration: "15s", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.01"],    // <1% errors
    http_req_duration: ["p(95)<200"],  // 95% under 200 ms
  },
};

export default function () {
  for (const path of ["/", "/login", "/privacy", "/sitemap.xml"]) {
    // note: the app limits each IP to 300 req/min – run k6 from several IPs or raise the limit for load tests
    const res = http.get(`${BASE}${path}`);
    check(res, { [`${path} 200`]: (r) => r.status === 200 });
  }
  sleep(1);
}
