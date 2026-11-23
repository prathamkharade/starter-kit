// Load test (k6). Run without installing anything:
//   docker run --rm -i --network host grafana/k6 run - < infra/k6/smoke.js
// Env: BASE_URL (default http://localhost:8080), VUS, DURATION.
// NOTE: the API rate-limits 300 req/min per IP. From one machine keep VUS low, or
// temporarily raise the limiter in internal/server/server.go for capacity tests.
import http from "k6/http";
import { check, sleep } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8080";

http.setResponseCallback(http.expectedStatuses(200, 401));

export const options = {
  vus: Number(__ENV.VUS || 1),
  duration: __ENV.DURATION || "30s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<300"],
  },
};

export default function () {
  const r1 = http.get(`${BASE}/healthz`);
  check(r1, { "health 200": (r) => r.status === 200 });
  const r2 = http.get(`${BASE}/v1/app/config`);
  check(r2, { "config 200": (r) => r.status === 200 });
  const r3 = http.get(`${BASE}/v1/notes`);
  check(r3, { "auth enforced": (r) => r.status === 401 });
  sleep(1);
}
