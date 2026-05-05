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

import SelectableButtonGroup from '../../../common/ui/SelectableButtonGroup';

/**
 * 分组筛选组件
 * @param {'all'|number} filterGroup 当前选中的分组，'all' 表示不过滤
 * @param {Function} setFilterGroup 设置选中分组
 * @param {Array<any>} userSelectableGroups 用户可选分组列表（从 /api/user/self/groups 获取）
 * @param {Record<string, number>} groupRatio 分组倍率对象（key 为 group_id）
 * @param {Record<string, string>} groupLabelById 分组显示名映射（key 为 group_id）
 * @param {Array} models 模型列表
 * @param {boolean} loading 是否加载中
 * @param {Function} t i18n
 */
const PricingGroups = ({
  filterGroup,
  setFilterGroup,
  userSelectableGroups = [],
  groupRatio = {},
  groupLabelById = {},
  models = [],
  loading = false,
  t,
}) => {
  const groupIds = Array.isArray(userSelectableGroups)
    ? userSelectableGroups
        .map((g) => Number(g?.id ?? 0))
        .filter((id) => Number.isFinite(id) && id > 0)
        .map((id) => Math.floor(id))
    : [];
  const uniqueGroupIds = Array.from(new Set(groupIds)).sort((a, b) => a - b);
  const groups = ['all', ...uniqueGroupIds];

  const items = groups.map((g) => {
    const modelCount =
      g === 'all'
        ? models.length
        : models.filter((m) => {
            const enableIds = Array.isArray(m?.enable_group_ids)
              ? m.enable_group_ids
              : [];
            return enableIds.some((id) => Number(id) === Number(g));
          }).length;
    let ratioDisplay = '';
    if (g === 'all') {
      ratioDisplay = t('全部');
    } else {
      const ratio = groupRatio[g];
      if (ratio !== undefined && ratio !== null) {
        ratioDisplay = `x${ratio}`;
      } else {
        ratioDisplay = 'x1';
      }
    }
    return {
      value: g,
      label: g === 'all' ? t('全部分组') : groupLabelById[g] || t('未知分组'),
      tagCount: ratioDisplay,
      disabled: modelCount === 0,
    };
  });

  return (
    <SelectableButtonGroup
      title={t('可用令牌分组')}
      items={items}
      activeValue={filterGroup}
      onChange={setFilterGroup}
      loading={loading}
      t={t}
    />
  );
};

export default PricingGroups;
