// Selects the API server at build time, defaulting to the public production API.

class AppConfig {
  static const apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'https://mosaicfantasy.com',
  );
}
