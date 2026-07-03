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
import React, { useEffect } from 'react';

// Fy-api overlay: 产品文档已合并为静态单页，/docs 入口跳转到平台功能指南标签。
const PRODUCT_DOCS_URL = '/product-docs/api-reference.html#platform';

export default function FyApiDocs() {
  useEffect(() => {
    window.location.replace(PRODUCT_DOCS_URL);
  }, []);

  return (
    <div style={{
      minHeight: '60vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: '48px 16px',
      color: 'rgb(31, 35, 41)',
    }}>
      <a href={PRODUCT_DOCS_URL}>正在打开平台功能指南...</a>
    </div>
  );
}
