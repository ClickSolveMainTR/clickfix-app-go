// google-services.json is not committed; EAS provides it as the file
// environment variable GOOGLE_SERVICES_JSON (production/preview builds).
module.exports = ({ config }) => ({
  ...config,
  android: {
    ...config.android,
    googleServicesFile: process.env.GOOGLE_SERVICES_JSON ?? config.android?.googleServicesFile,
  },
});
