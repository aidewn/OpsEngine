// Toast 封装：统一应用内成功、失败和提示反馈。
import { toast as baseToast } from 'sonner';

type ToastAction = {
  label: string;
  onClick: () => void;
};

function withAction(action?: ToastAction) {
  return action
    ? {
        action: {
          label: action.label,
          onClick: action.onClick,
        },
      }
    : undefined;
}

export const toast = {
  success(message: string, action?: ToastAction) {
    baseToast.success(message, withAction(action));
  },
  error(message: string, description?: string) {
    baseToast.error(message, { description });
  },
  info(message: string, action?: ToastAction) {
    baseToast.info(message, withAction(action));
  },
};
