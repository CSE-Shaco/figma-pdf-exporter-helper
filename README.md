# Figma PDF Exporter 로컬 글꼴 도우미

[한국어](#한국어) · [English](#english)

## 한국어

Figma PDF Exporter Pro 또는 Preview에서 컴퓨터에 설치된 글꼴을 찾아 PDF에 사용할 수 있도록 돕는 도우미입니다.

### 다운로드 및 설치

[v0.1.0 릴리스](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/tag/v0.1.0)에서 컴퓨터에 맞는 ZIP을 내려받으세요.

| 컴퓨터 | 다운로드 |
| --- | --- |
| Apple Silicon Mac (M 시리즈) | [macOS ARM64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-macos-arm64.zip) |
| Intel Mac | [macOS x86_64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-macos-x86_64.zip) |
| Windows x64 | [Windows x64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-windows-x64.zip) |

1. ZIP 전체를 압축 해제하세요. 내부 `dist` 폴더와 실행 파일을 함께 유지하세요.
2. Mac은 `start-macos.command`, Windows는 `start-windows.cmd`를 실행하세요. Python 설치는 필요하지 않습니다.
3. 창에 표시된 **64자리 16진수 연결 코드**를 PDF Exporter Pro 또는 Preview의 **Local Fonts** 영역에 입력하세요.
4. 플러그인에서 글꼴을 검색하세요. 사용하는 동안 도우미 창을 열어 두세요. 종료하려면 창을 닫거나 `Ctrl+C`를 누르세요.

**이 릴리스는 서명·공증되지 않았습니다.** macOS Gatekeeper나 Windows SmartScreen이 실행을 제한할 수 있습니다. 출처와 [SHA-256 체크섬](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/SHA256SUMS.txt)을 확인하세요. Mac에서 확인 후 실행하려면 Apple의 [안전한 앱 열기 안내](https://support.apple.com/en-us/102445)에 따라 시스템 설정 → 개인정보 보호 및 보안에서 해당 앱의 실행을 허용하세요. 조직의 보안 정책으로 제한될 때는 관리자에게 문의하세요.

### 지원 범위와 검증

- Intel Mac과 Apple Silicon Mac을 별도 네이티브 빌드로 제공합니다. macOS 15 CI에서 실행·OS 글꼴 검색·인증·다운로드 바이트의 SHA-256 검증을 통과했습니다. 다른 macOS 버전은 별도 검증하지 않았습니다.
- Windows x64 실행 파일은 Windows Server 2022 CI에서 같은 실행·인증·글꼴 다운로드 검증을 통과했습니다. Windows 10/11 데스크톱에서의 수동 사용 검증은 별도입니다. Windows ARM64 빌드는 제공하지 않습니다.
- macOS CoreText 및 Windows 글꼴 등록/표준 폴더에 노출된 읽기 가능한 SFNT/TTC 글꼴을 찾습니다. 확장자 없는 활성화 글꼴도 실제 헤더로 확인합니다. WOFF/WOFF2는 지원하지 않습니다.
- 단일 글꼴은 20 MB, 글꼴 컬렉션은 64 MB까지 읽습니다. 플러그인의 글꼴 이름·스타일·문자 포함 여부·PDF 임베딩 제한 검사도 적용됩니다.
- Adobe 출처를 이유로 제외하지 않습니다. Creative Cloud에서 다른 앱에서도 사용할 수 있도록 글꼴을 활성화한 뒤 도우미를 다시 실행하세요. 실제 Creative Cloud 활성화 환경은 미검증이며, Windows OS 글꼴 등록 밖의 CC 글꼴은 검색되지 않을 수 있습니다. 비공개 캐시나 보호된 컨테이너는 탐색하거나 해독하지 않습니다. 글꼴 사용 권한과 허용된 사용 방식은 사용자가 확인해야 합니다.

도우미는 로컬 컴퓨터의 루프백 인터페이스에서만 통신하며 연결 코드로 인증합니다. 플러그인은 필요한 글꼴 패밀리 이름을 요청하고 글꼴 데이터를 받습니다. 문서 텍스트는 전송하지 않습니다. 설치 글꼴을 바꾸면 도우미를 재실행해 목록을 갱신하세요.

설치·실행 문제는 [Issues](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/issues)에 등록하세요. 연결 코드, 토큰, 글꼴 파일, 비공개 정보를 올리지 마세요.

## English

This helper lets Figma PDF Exporter Pro or Preview find locally installed fonts for PDF export.

### Download and setup

Download the ZIP for your computer from [v0.1.0](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/tag/v0.1.0):

| Computer | Download |
| --- | --- |
| Apple Silicon Mac (M series) | [macOS ARM64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-macos-arm64.zip) |
| Intel Mac | [macOS x86_64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-macos-x86_64.zip) |
| Windows x64 | [Windows x64](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/font-helper-windows-x64.zip) |

1. Extract the entire ZIP, keeping its `dist` directory beside the launcher.
2. Run `start-macos.command` on Mac or `start-windows.cmd` on Windows. Python installation is not required.
3. Paste the **64-character hexadecimal pairing code** into **Local Fonts** in PDF Exporter Pro or Preview.
4. Search fonts in the plugin. Keep the helper window open while using it; close the window or press `Ctrl+C` to stop.

**This release has no publisher signature or notarization.** macOS Gatekeeper or Windows SmartScreen may block it. Verify the source and [SHA-256 checksums](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/latest/download/SHA256SUMS.txt). On Mac, follow [Apple’s guidance](https://support.apple.com/en-us/102445) to allow the specific app in System Settings → Privacy & Security after checking it. Contact your administrator if organizational policy blocks execution.

### Support and validation

- Separate native Intel and Apple Silicon Mac builds pass startup, OS font discovery, authentication and downloaded-byte SHA-256 checks in macOS 15 CI. Other macOS versions have not been separately verified.
- The Windows x64 executable passes equivalent checks in Windows Server 2022 CI. Manual use on Windows 10/11 desktops remains unverified. No Windows ARM64 build is provided.
- Readable SFNT/TTC fonts exposed by macOS CoreText or Windows font registrations/standard directories are indexed by signature, including extensionless activated fonts. WOFF/WOFF2 are not supported.
- Standalone fonts are limited to 20 MB, collections to 64 MB. The plugin also checks font names, styles, glyph coverage and PDF embedding restrictions.
- Adobe Fonts are not excluded based on publisher. Activate fonts for other apps in Creative Cloud, then restart the helper. Actual Creative Cloud activation has not been verified; Windows CC fonts outside OS registrations may not be discovered. Private caches and protected containers are not scanned or decoded. Font licensing and permitted use remain your responsibility.

The helper listens only on the local loopback interface and authenticates with the pairing code. The plugin requests font family names and receives font bytes; document text is not sent. Restart the helper after changing installed fonts.

Use [Issues](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/issues) for installation or startup problems. Do not include pairing codes, tokens, font files or private information.

<details>
<summary>개발자용: 소스에서 직접 빌드 / For developers: Build from source</summary>

일반 사용자는 위 ZIP 다운로드만 하면 됩니다. 이 단계는 도우미 코드를 수정하거나 실행 파일을 직접 만드는 개발자용입니다.
Regular users only need the ZIP downloads above. These steps are for developers modifying the helper or producing executables themselves.

macOS는 대상 CPU에서 네이티브로 실행되는 Python 3.9+가 필요합니다. CI는 Python 3.13과 별도 네이티브 runner를 사용합니다. Windows에서는 PATH의 Python을 사용하며 `PYTHON_BIN`으로 다른 실행 파일을 지정할 수 있습니다.
macOS builds require Python 3.9+ running natively on the target CPU. CI uses Python 3.13 and separate native runners. Windows uses Python on PATH; set `PYTHON_BIN` to choose another interpreter.

```sh
python3 -m unittest discover -s . -p 'test_*.py'
# Intel Mac
TARGET_ARCH=x86_64 ./build-macos.sh
# Apple Silicon
TARGET_ARCH=arm64 ./build-macos.sh
python3 smoke-binary.py dist/font-helper-macos-$(python3 -c 'import platform; print(platform.machine())')
```

Windows: `build-windows.cmd`, then `python smoke-binary.py dist/font-helper-windows.exe`.

빌드는 PyInstaller 6.22.3을 사용합니다. Actions의 **Build local font helper**는 테스트 artifact만 업로드하고 자동으로 릴리스를 게시하지 않습니다. Artifact는 14일 뒤 만료됩니다. 고객 다운로드에는 Releases를 사용하세요. ZIP의 `licenses` 폴더에는 번들 런타임의 제3자 고지가 포함됩니다.
Builds use PyInstaller 6.22.3. **Build local font helper** uploads CI test artifacts only; it does not automatically publish releases. Artifacts expire after 14 days. Use Releases for customer downloads. The ZIP’s `licenses` directory contains third-party runtime notices.

</details>
