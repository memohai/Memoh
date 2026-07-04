import { app, dialog } from 'electron'
import { is } from '@electron-toolkit/utils'

// macOS 双击自安装:DMG 里只放 app,用户双击图标启动时,若发现自己不在
// /Applications,就把自己拷进去再重启 —— 免去"拖到 Applications"那一步。
//
// 为什么必须在 whenReady 的最前面调用(早于 ensureLocalServer):
//   首次启动时 app 跑在只读的 DMG 卷(甚至 App Translocation 的随机只读路径)。
//   若先拉起本地 server / Qdrant 再搬家,会从 DMG 路径 spawn 出子进程,搬家重启后
//   留下指向已卸载卷的孤儿进程。所以自安装是启动序列的第 0 步:要么搬家+重启,
//   要么原地继续 —— 二者必居其一,且在任何本地进程被拉起之前决定。

const MOVE_CONFLICT_TITLE = 'Memoh is already installed'

/**
 * 若在 macOS 打包态且当前不在 /Applications,尝试把 app 搬进 /Applications。
 *
 * @returns true 表示已触发搬家 + 重启,调用方应立即 return、不要再启动任何本地进程。
 *          false 表示无需搬家 / 搬家失败 / 用户取消 —— 调用方按原地运行继续。
 */
export function maybeSelfInstallMacOS(): boolean {
  // 仅打包态的 macOS 才自安装。dev 下 app 路径本就不在 /Applications,不能动。
  if (process.platform !== 'darwin' || is.dev || !app.isPackaged) return false

  // 已在 /Applications(第二次及以后启动)-> 什么都不做,正常进。
  let alreadyInstalled = true
  try {
    alreadyInstalled = app.isInApplicationsFolder()
  } catch {
    // 某些沙盒/权限异常下该 API 可能抛;保守当作已安装,绝不阻塞启动。
    return false
  }
  if (alreadyInstalled) return false

  try {
    const moved = app.moveToApplicationsFolder({
      conflictHandler: (conflictType) => {
        // 已存在同名 app:让用户决定覆盖还是就地运行当前这份。
        // conflictType 取值是 'exists' / 'existsAndRunning'(Electron API 原文)。
        if (conflictType === 'exists') {
          const response = dialog.showMessageBoxSync({
            type: 'question',
            buttons: ['Replace', 'Keep Both / Run Here'],
            defaultId: 0,
            cancelId: 1,
            title: MOVE_CONFLICT_TITLE,
            message: MOVE_CONFLICT_TITLE,
            detail:
              'A version of Memoh already exists in your Applications folder. Replace it with this one?',
          })
          // 返回 true = 继续搬家(丢弃旧版、装入这份);false = 放弃,原地运行。
          return response === 0
        }
        // 'existsAndRunning':旧版正在运行,无法安全覆盖 —— 放弃搬家,原地运行。
        return false
      },
    })
    // moved===true 时 Electron 会拷贝到 /Applications、启动那一份并退出当前实例。
    return moved
  } catch (error) {
    // 搬家失败(权限/磁盘/只读目标等)绝不能卡死首启 —— 吞掉,原地运行。
    console.error('self-install: moveToApplicationsFolder failed', error)
    return false
  }
}
