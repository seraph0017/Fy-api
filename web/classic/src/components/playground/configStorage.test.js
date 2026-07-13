/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import test from 'node:test';
import {
  DEFAULT_CONFIG,
  STORAGE_KEYS,
} from '../../constants/playground.constants.js';
import { loadConfig } from './configStorage.js';

function installLocalStorageMock(initialValues = {}) {
  const store = new Map(Object.entries(initialValues));

  globalThis.localStorage = {
    getItem(key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem(key, value) {
      store.set(key, String(value));
    },
    removeItem(key) {
      store.delete(key);
    },
  };
}

test('default playground parameters are all disabled by default', () => {
  assert.deepEqual(DEFAULT_CONFIG.parameterEnabled, {
    temperature: false,
    top_p: false,
    max_tokens: false,
    frequency_penalty: false,
    presence_penalty: false,
    seed: false,
  });
});

test('loadConfig migrates legacy default optional parameters to disabled', () => {
  installLocalStorageMock({
    [STORAGE_KEYS.CONFIG]: JSON.stringify({
      inputs: {
        ...DEFAULT_CONFIG.inputs,
        temperature: 0.7,
        top_p: 1,
        frequency_penalty: 0,
        presence_penalty: 0,
      },
      parameterEnabled: {
        temperature: true,
        top_p: true,
        max_tokens: false,
        frequency_penalty: true,
        presence_penalty: true,
        seed: false,
      },
    }),
  });

  const config = loadConfig();

  assert.equal(config.parameterEnabled.temperature, false);
  assert.equal(config.parameterEnabled.top_p, false);
  assert.equal(config.parameterEnabled.frequency_penalty, false);
  assert.equal(config.parameterEnabled.presence_penalty, false);
});

test('loadConfig keeps legacy optional parameters enabled when values were customized', () => {
  installLocalStorageMock({
    [STORAGE_KEYS.CONFIG]: JSON.stringify({
      inputs: {
        ...DEFAULT_CONFIG.inputs,
        temperature: 0.2,
        top_p: 0.8,
        frequency_penalty: 0.3,
        presence_penalty: 0.2,
      },
      parameterEnabled: {
        temperature: true,
        top_p: true,
        max_tokens: false,
        frequency_penalty: true,
        presence_penalty: true,
        seed: false,
      },
    }),
  });

  const config = loadConfig();

  assert.equal(config.parameterEnabled.temperature, true);
  assert.equal(config.parameterEnabled.top_p, true);
  assert.equal(config.parameterEnabled.frequency_penalty, true);
  assert.equal(config.parameterEnabled.presence_penalty, true);
});
