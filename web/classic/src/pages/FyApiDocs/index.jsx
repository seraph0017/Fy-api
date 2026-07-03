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
import React from 'react';

// Fy-api overlay: 产品文档已合并为静态单页，/docs 入口跳转到平台功能指南标签。
const PRODUCT_DOCS_URL = '/product-docs/api-reference.html#platform';

export default function FyApiDocs() {
  return (
    <iframe
      src={PRODUCT_DOCS_URL}
      title="产品文档"
      style={{
        width: '100%',
        height: '100vh',
        border: 'none',
        position: 'absolute',
        top: 0,
        left: 0,
      }}
    />
  );
}
