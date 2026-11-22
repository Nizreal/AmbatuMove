const { getDefaultConfig } = require("expo/metro-config");
const { createDevApiProxy } = require("./scripts/dev-api-proxy.cjs");

const config = getDefaultConfig(__dirname);
const previousEnhancer = config.server.enhanceMiddleware;

config.server.enhanceMiddleware = (middleware, server) => {
  const enhancedMiddleware = previousEnhancer(middleware, server);
  const apiProxy = createDevApiProxy();

  return (request, response, next) => {
    if (apiProxy(request, response)) return;
    return enhancedMiddleware(request, response, next);
  };
};

module.exports = config;
