import { h } from 'vue'
import { useDialog, useMessage, useNotification } from 'naive-ui'
import { api, errMsg, type PowerAction } from '@/api'
import { powerActions } from '@/utils/format'

// Power, console and identify actions with confirmation and feedback,
// shared by the device list and the device detail page.
export function useDeviceActions() {
  const dialog = useDialog()
  const message = useMessage()
  const notification = useNotification()

  function power(ids: number[], action: PowerAction, names: string[]): Promise<boolean> {
    const meta = powerActions.find((a) => a.key === action)!
    const target = ids.length === 1 ? `“${names[0]}”` : `所选 ${ids.length} 台设备`
    return new Promise((resolve) => {
      dialog[meta.danger ? 'warning' : 'info']({
        title: `${meta.label} · ${target}`,
        content: meta.confirm,
        positiveText: meta.label,
        negativeText: '取消',
        positiveButtonProps: meta.danger ? { type: 'error' } : undefined,
        onNegativeClick: () => resolve(false),
        onClose: () => resolve(false),
        // ESC and mask clicks close the dialog without the callbacks above;
        // resolving again after a button is a no-op.
        onAfterLeave: () => resolve(false),
        onPositiveClick: async () => {
          try {
            const res = await api.bmc.power(ids, action)
            if (res.failed === 0) {
              message.success(`${meta.label}指令已发送（${res.succeeded} 台）`)
            } else {
              notification.warning({
                title: `${meta.label}：${res.succeeded} 台成功，${res.failed} 台失败`,
                content: () =>
                  h('div', { style: 'max-height:240px;overflow:auto;font-size:12.5px' },
                    res.items.filter((i) => !i.ok).map((i) => h('div', { style: 'margin:4px 0' }, [h('b', i.name), '：', i.error]))),
                duration: 0,
              })
            }
            resolve(true)
          } catch (e) {
            message.error(errMsg(e))
            resolve(false)
          }
        },
      })
    })
  }

  async function openWeb(id: number) {
    const m = message.loading('正在打开 BMC Web 管理…', { duration: 0 })
    try {
      const r = await api.bmc.openWeb(id)
      m.destroy()
      message.success(r.autoLogin ? '已打开 Web 管理窗口（自动登录）' : '已打开 BMC 登录页')
    } catch (e) {
      m.destroy()
      message.error(errMsg(e), { duration: 6000 })
    }
  }

  async function openKVM(id: number) {
    const m = message.loading('正在打开远程控制台…', { duration: 0 })
    try {
      const r = await api.bmc.openKVM(id)
      m.destroy()
      if (r.hint) message.info(r.hint, { duration: 8000 })
      else message.success('已打开远程控制台窗口')
    } catch (e) {
      m.destroy()
      message.error(errMsg(e), { duration: 6000 })
    }
  }

  async function identify(id: number, on: boolean) {
    try {
      await api.bmc.identifyLED(id, on)
      message.success(on ? '定位灯已点亮' : '定位灯已关闭')
    } catch (e) {
      message.error(errMsg(e))
    }
  }

  return { power, openWeb, openKVM, identify }
}
