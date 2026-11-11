// Same setup as @getclickfix/react-native; the Expo modules are replaced by
// in-memory doubles (tests/mocks) so nothing native is needed.
module.exports = {
  preset: '@react-native/jest-preset',
  testMatch: ['<rootDir>/tests/**/*.test.ts?(x)'],
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
    '^@getclickfix/visitor-core$': '<rootDir>/../visitor-core/src/index.ts',
    '^@getclickfix/react-native$': '<rootDir>/../react-native/src/index.ts',
    '^@getclickfix/react-native/ui$': '<rootDir>/../react-native/src/ui/index.ts',
    '^expo-secure-store$': '<rootDir>/tests/mocks/expo-secure-store.ts',
    '^expo-file-system$': '<rootDir>/tests/mocks/expo-file-system.ts',
    '^expo-linking$': '<rootDir>/tests/mocks/expo-linking.ts',
    '^expo-constants$': '<rootDir>/tests/mocks/expo-constants.ts',
  },
  transformIgnorePatterns: ['node_modules/(?!\\.pnpm/)(?!((jest-)?react-native|@react-native(-community)?)/)', 'node_modules/\\.pnpm/(?!((jest-)?react-native|@react-native(-community)?)[@+])'],
  setupFilesAfterEnv: ['<rootDir>/../react-native/tests/setup.ts'],
};
