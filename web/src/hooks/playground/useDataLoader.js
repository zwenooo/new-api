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

import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { API, processModelsData, processGroupsData } from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
) => {
  const { t } = useTranslation();

  const loadModels = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_MODELS);
      const { success, message, data } = res.data;

      if (success) {
        const { modelOptions, selectedModel } = processModelsData(
          data,
          inputs.model,
        );
        setModels(modelOptions);

        if (selectedModel !== inputs.model) {
          handleInputChange('model', selectedModel);
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载模型失败'));
    }
  }, [inputs.model, handleInputChange, setModels, t]);

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS);
      const { success, message, data } = res.data;

      if (success) {
        const cachedUser = (() => {
          try {
            return JSON.parse(localStorage.getItem('user'));
          } catch {
            return null;
          }
        })();
        const userGroupId =
          Number(userState?.user?.group_id ?? cachedUser?.group_id ?? 0) || 0;
        const groupOptions = processGroupsData(data, userGroupId);
        setGroups(groupOptions);

        const hasCurrentGroup = groupOptions.some(
          (option) => Number(option.value) === Number(inputs.group_id),
        );
        if (!hasCurrentGroup) {
          handleInputChange('group_id', Number(groupOptions[0]?.value ?? 0) || 0);
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载分组失败'));
    }
  }, [userState, inputs.group_id, handleInputChange, setGroups, t]);

  // 自动加载数据
  useEffect(() => {
    if (userState?.user) {
      loadModels();
      loadGroups();
    }
  }, [userState?.user, loadModels, loadGroups]);

  return {
    loadModels,
    loadGroups,
  };
};
