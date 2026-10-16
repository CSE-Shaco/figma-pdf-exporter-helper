# Figma PDF Exporter 로컬 글꼴 도우미

[한국어](#한국어) · [English](#english)

## 한국어

Figma PDF Exporter에서 컴퓨터에 설치된 글꼴을 찾고 PDF에 사용할 수 있도록 돕는 로컬 도우미의 다운로드 및 설치 안내입니다.

이 저장소에는 독립 실행 가능한 도우미 소스와 Intel Mac·Apple Silicon·Windows 빌드 자동화가 있습니다. **현재 고객용 다운로드 파일이나 공개 릴리스는 없습니다.** 이 저장소에 릴리스가 게시되기 전까지는 다운로드할 수 있는 것으로 간주하지 마세요.

### 설치 예정 순서

릴리스가 게시되면 운영체제에 맞는 압축 파일을 이 저장소의 Releases 페이지에서 내려받아 압축을 푼 다음, 압축 파일에 포함된 실행 명령을 사용하세요.

- macOS: `start-macos.command`
- Windows: `start-windows.cmd`

도우미가 표시하는 **64자리 16진수 연결 코드**를 PDF Exporter Pro 또는 Preview의 Local Fonts 영역에 입력한 다음, 플러그인에서 컴퓨터에 설치된 글꼴을 검색하세요.

도우미는 컴퓨터 내부의 루프백 인터페이스에서 통신합니다. 글꼴 검색 요청은 로컬 컴퓨터에 머뭅니다. 플러그인은 필요한 글꼴 패밀리 이름을 요청하고, 인증된 루프백 연결을 통해 일치하는 글꼴 데이터를 받습니다. 문서 텍스트는 전송되지 않습니다. 연결에는 비공개 토큰을 사용하며, 기존 글꼴 접근 검사는 그대로 적용됩니다.

### 제공 상태와 제한

- Apple Silicon용 macOS 도우미가 있지만, 현재 서명되지 않았고 공증도 받지 않았습니다.
- Intel Mac용 `x86_64` 빌드와 CPU 자동 선택 런처를 구현했습니다. 실제 Intel 바이너리 빌드·실행 검증은 GitHub Actions 실행 결과를 확인해야 합니다.
- Windows PowerShell 도우미는 있지만, 패키지된 실행 파일은 아직 빌드하거나 검증하지 않았습니다.
- macOS CoreText 또는 Windows 글꼴 등록을 통해 운영체제에 노출된 읽기 가능한 SFNT/TTC 글꼴을 지원합니다. 활성화된 글꼴에 파일 확장자가 없어도 찾을 수 있습니다. Adobe 글꼴도 제작사만을 이유로 제외하지 않습니다. 실제 Creative Cloud 활성화 환경에서는 확인하지 않았으며, Windows에서 운영체제 글꼴 등록 밖에 있는 CC 글꼴은 검색되지 않을 수 있습니다. 비공개 캐시나 보호된 컨테이너는 검색하거나 해독하지 않습니다. 글꼴 사용 권한과 허용된 사용 방식은 사용자가 확인해야 하며, 기존 PDF 임베딩 검사도 계속 적용됩니다.

제공 상태와 제한은 바뀔 수 있습니다. 공개 빌드를 설치하기 전에 릴리스 노트를 확인하세요.

### 지원

도우미 설치나 실행 문제는 이 저장소의 Issues에 등록해 주세요. 이슈에 연결 코드, 접근 토큰, 글꼴 파일 또는 기타 비공개 정보를 포함하지 마세요.

## English

Local font helper downloads and installation guide for Figma PDF Exporter.

This repository contains the standalone helper source and automated Intel Mac, Apple Silicon and Windows builds. **No customer download or published release is available yet.** Do not treat a future release link as available until a release is published here.

### Planned setup

When a release is available, choose the archive for your operating system, download it from this repository's Releases page, extract it, and start the helper with the operating-system-specific command included in the archive:

- macOS: `start-macos.command`
- Windows: `start-windows.cmd`

The helper displays a 64-character hexadecimal pairing code. Enter that code in the Local Fonts area of PDF Exporter Pro or Preview, then use the plugin to search fonts installed on your computer.

The helper is designed to listen on the local loopback interface. Font search requests stay on the local machine; the plugin requests needed font family names and receives matching font bytes over authenticated loopback. Document text is not sent. Pairing uses a private token, and the existing font access checks remain in effect.

### Availability and limitations

- A macOS Apple Silicon helper exists, but it is currently unsigned and has not been notarized.
- Native Intel Mac (`x86_64`) builds and CPU-aware launching are implemented. Actual Intel binary build and execution verification still requires a successful GitHub Actions run.
- A Windows PowerShell helper exists, but a packaged executable has not been built or validated.
- Readable SFNT/TTC fonts exposed by macOS CoreText or Windows font registrations are supported even when activated fonts have no filename extension. Adobe Fonts are not excluded based on publisher. Actual Creative Cloud activation has not been verified; Windows CC fonts outside OS registrations may not be discovered. Private caches and protected containers are not scanned or decoded. Font licensing and permitted use remain the user’s responsibility, and existing PDF embedding checks still apply.

These limitations may change. Check the release notes before installing a published build.

### Support

Please use this repository's Issues for helper installation and startup problems. Do not include pairing codes, access tokens, font files, or other private data in an issue.

## 소스에서 빌드 / Build from source

Python 3.9+가 설치된 해당 CPU의 Mac에서 실행하세요. GitHub Actions는 Python 3.13과 CPU별 네이티브 runner를 사용합니다.
Use a Mac with Python 3.9+ running natively on the target CPU. CI uses Python 3.13 on separate native runners.

```sh
python3 -m unittest discover -s . -p 'test_*.py'
# Intel Mac
TARGET_ARCH=x86_64 ./build-macos.sh
# Apple Silicon
TARGET_ARCH=arm64 ./build-macos.sh
python3 smoke-binary.py dist/font-helper-macos-$(python3 -c 'import platform; print(platform.machine())')
./start-macos.command
```

다른 CPU의 Python으로 빌드하면 오류로 중단합니다. `PYTHON_BIN`으로 해당 CPU의 Python 실행 파일을 지정할 수 있습니다. 런처는 Intel에서 `dist/font-helper-macos-x86_64`, Apple Silicon에서 `dist/font-helper-macos-arm64`를 선택합니다. 호환되는 기존 `dist/font-helper-macos`와 Python 소스 실행도 지원합니다.
The build rejects a Python interpreter running on a different CPU. Set `PYTHON_BIN` to select the native interpreter. The launcher selects the CPU-specific binary and retains compatible legacy binary and Python source fallbacks.

Actions → **Build local font helper** → **Run workflow**에서 빌드할 수 있습니다. 성공하면 `font-helper-macos-x86_64-ci`, `font-helper-macos-arm64-ci`, `font-helper-windows-ci` 테스트 artifact가 생성됩니다. macOS artifact는 런처·바이너리·README를 포함한 ZIP 및 SHA-256입니다. CI artifact는 14일 후 만료되며 고객용 Releases 다운로드가 아닙니다. 빌드 자동화는 공개 릴리스를 게시하지 않습니다.
Run **Build local font helper** in Actions to create separate test artifacts. macOS ZIPs include the launcher, executable, README and a SHA-256 file. Artifacts expire after 14 days; they are not customer releases. The workflow never publishes a release.
