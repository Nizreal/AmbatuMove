// This middleware is CommonJS because Metro loads its configuration in Node.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const { createDevApiProxy } = require("../../scripts/dev-api-proxy.cjs") as {
  createDevApiProxy: (
    request: (...args: any[]) => any,
  ) => (req: any, res: any) => boolean;
};

beforeEach(() => {
  jest.spyOn(console, "info").mockImplementation(() => {});
});
afterEach(() => jest.restoreAllMocks());

function requestFor(
  url: string,
  method = "GET",
  body = "",
  headers: Record<string, string> = { accept: "application/json" },
) {
  const requestMock = jest.fn((target, options, receiveResponse) => {
    const upstreamRequest = {
      setTimeout: jest.fn(),
      on: jest.fn(),
      destroy: jest.fn(),
      end: jest.fn((receivedBody = "") => {
        receiveResponse({
          statusCode: 200,
          headers: { "content-type": "application/json" },
          pipe: (response: any) => {
            response.writeHead(200, { "content-type": "application/json" });
            response.end(
              JSON.stringify({
                method: options.method,
                url: `${target.origin}${target.pathname}${target.search}`,
                body: receivedBody,
              }),
            );
          },
        });
      }),
    };
    return upstreamRequest;
  });
  const proxy = createDevApiProxy(requestMock);
  const req = {
    url,
    method,
    headers,
    on: jest.fn(),
    pipe: (upstreamRequest: { end: (body: string) => void }) =>
      upstreamRequest.end(body),
  };
  const res = {
    headersSent: false,
    writeHead: jest.fn(),
    end: jest.fn(),
    destroy: jest.fn(),
  };
  expect(proxy(req, res)).toBe(true);
  return { requestMock, res };
}

test("development proxy targets the deployed API and preserves query pagination", () => {
  const { requestMock, res } = requestFor(
    "/__dev_proxy/ambatumove/exercises?limit=100&offset=200",
  );
  const [target, options] = requestMock.mock.calls[0] as unknown as [
    URL,
    { method: string },
  ];
  expect(target.href).toBe(
    "https://nizdri.stei.fun/api/v1/exercises?limit=100&offset=200",
  );
  expect(options.method).toBe("GET");
  expect(res.writeHead).toHaveBeenCalledWith(200, {
    "content-type": "application/json",
  });
  expect(JSON.parse(res.end.mock.calls[0][0])).toMatchObject({
    method: "GET",
    url: "https://nizdri.stei.fun/api/v1/exercises?limit=100&offset=200",
  });
});

test("development proxy forwards JSON workout preview requests without browser preflight", () => {
  const body = JSON.stringify({ save: false });
  const { requestMock, res } = requestFor(
    "/__dev_proxy/ambatumove/workouts/generate",
    "POST",
    body,
  );
  expect(requestMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
  expect(JSON.parse(res.end.mock.calls[0][0])).toMatchObject({
    method: "POST",
    url: "https://nizdri.stei.fun/api/v1/workouts/generate",
    body,
  });
});

test("development proxy maps the AmbatuFit directory to its own upstream", () => {
  const { requestMock, res } = requestFor(
    "/__dev_proxy/ambatufit/professionals",
  );
  expect(requestMock.mock.calls[0][0].origin).toBe(
    "https://ambatufit.stei.fun",
  );
  expect(JSON.parse(res.end.mock.calls[0][0]).url).toBe(
    "https://ambatufit.stei.fun/api/v1/professionals",
  );
});

test("development proxy preserves API authentication headers", () => {
  const { requestMock } = requestFor("/__dev_proxy/ambatumove/me", "GET", "", {
    accept: "application/json",
    authorization: "Bearer test-token",
    "x-api-key": "test-partner-key",
  });
  expect(requestMock.mock.calls[0][1].headers).toMatchObject({
    authorization: "Bearer test-token",
    "x-api-key": "test-partner-key",
  });
});
