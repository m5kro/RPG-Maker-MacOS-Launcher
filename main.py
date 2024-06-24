# m5kro - 2024

import sys
import os
import json
import subprocess
import requests
import zipfile
import shutil
import platform
import chardet
import logging
from evbunpack.__main__ import unpack_files
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QHBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QFormLayout, QLabel, QCheckBox)

# Set up logging
LOG_FILE = os.path.expanduser("~/Applications/RPGM-Launcher/log.txt")
logging.basicConfig(filename=LOG_FILE, level=logging.DEBUG, 
                    format='%(asctime)s - %(levelname)s - %(message)s')

class FolderPathApp(QMainWindow):
    SETTINGS_FILE = os.path.expanduser("~/Applications/RPGM-Launcher/settings.json")

    def __init__(self):
        super().__init__()

        self.setWindowTitle("RPG Maker MV and MZ Launcher")
        self.resize(400, 200)

        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)

        self.layout = QVBoxLayout(self.central_widget)

        version_layout = QHBoxLayout()
        self.version_label = QLabel("NWJS version:", self)
        version_layout.addWidget(self.version_label)

        self.version_selector = QComboBox(self)
        version_layout.addWidget(self.version_selector)
        self.layout.addLayout(version_layout)

        self.extract_checkbox = QCheckBox("Extract game_en.exe", self)
        self.layout.addWidget(self.extract_checkbox)

        self.select_button = QPushButton("Select Game Folder", self)
        self.layout.addWidget(self.select_button)

        self.install_button = QPushButton("Install NWJS Version", self)
        self.layout.addWidget(self.install_button)

        self.uninstall_button = QPushButton("Uninstall NWJS Version", self)
        self.layout.addWidget(self.uninstall_button)

        self.install_button.clicked.connect(self.install_nwjs)
        self.uninstall_button.clicked.connect(self.uninstall_nwjs)
        self.select_button.clicked.connect(self.select_folder)

        self.update_version_selector()
        self.update_select_button_state()
        self.load_settings()

    def load_settings(self):
        if os.path.exists(self.SETTINGS_FILE):
            with open(self.SETTINGS_FILE, 'r') as file:
                settings = json.load(file)
                self.extract_checkbox.setChecked(settings.get('extract_game_en', False))
                last_version = settings.get('last_selected_version')
                if last_version and last_version in [self.version_selector.itemText(i) for i in range(self.version_selector.count())]:
                    self.version_selector.setCurrentText(last_version)

    def save_settings(self):
        settings = {
            'extract_game_en': self.extract_checkbox.isChecked(),
            'last_selected_version': self.version_selector.currentText()
        }
        with open(self.SETTINGS_FILE, 'w') as file:
            json.dump(settings, file, indent=4)

    def closeEvent(self, event):
        self.save_settings()
        super().closeEvent(event)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        return os.path.exists(applications_dir) and any(os.path.isdir(os.path.join(applications_dir, v)) for v in os.listdir(applications_dir))

    def update_version_selector(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        if os.path.exists(applications_dir):
            versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v))]
            self.version_selector.clear()
            self.version_selector.addItems(versions)
        self.update_select_button_state()

    def update_select_button_state(self):
        if self.check_nwjs_installed():
            self.select_button.setEnabled(True)
            self.select_button.setText("Select Game Folder")
        else:
            self.select_button.setEnabled(False)
            self.select_button.setText("NWJS not installed")

    def select_folder(self):
        folder_path = QFileDialog.getExistingDirectory(self, "Select Game Folder")
        if folder_path:
            logging.info("Selected folder path: %s", folder_path)
            if self.check_package_json(folder_path):
                logging.info("Valid RPG Maker MV/MZ game folder.")
                if self.extract_checkbox.isChecked():
                    self.check_and_unpack_game_en(folder_path)
                self.launch_game(folder_path)
            else:
                logging.error("No package.json found in the selected folder.")
                QMessageBox.critical(self, "Error", "No package.json found in the selected folder.")
        self.save_settings()

    def check_package_json(self, folder_path):
        file_path = os.path.join(folder_path, 'package.json')
        if os.path.exists(file_path):
            with open(file_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']

            with open(file_path, 'r', encoding=encoding) as file:
                data = json.load(file)

            if 'name' in data and data['name'].strip() == '':
                data['name'] = 'tempname'

            with open(file_path, 'w', encoding=encoding) as file:
                json.dump(data, file, indent=4)

            logging.info("JSON file updated successfully.")
            return True
        else:
            logging.error("File does not exist.")
            return False

    def check_and_unpack_game_en(self, folder_path):
        game_exe = None

        for root, _, files in os.walk(folder_path):
            for file in files:
                if file.lower() == "game_en.exe":
                    game_exe = os.path.join(root, file)
                    break
            if game_exe:
                break

        if game_exe:
            extracted_dir = os.path.join(folder_path, "extracted")
            unpack_files(game_exe, extracted_dir, False, False)

            for item in os.listdir(extracted_dir):
                s = os.path.join(extracted_dir, item)
                d = os.path.join(folder_path, item)
                if os.path.isdir(s):
                    shutil.copytree(s, d, dirs_exist_ok=True)
                else:
                    shutil.copy2(s, d)

            os.remove(game_exe)
            shutil.rmtree(extracted_dir)
            logging.info("game_en.exe extracted and original file removed.")
        else:
            logging.error("game_en.exe not found in the directory")

    def launch_game(self, folder_path):
        selected_version = self.version_selector.currentText()
        if not selected_version:
            logging.error("No NWJS version selected.")
            QMessageBox.critical(self, "Error", "No NWJS version selected.")
            return

        nwjs_dir = os.path.expanduser(f"~/Applications/RPGM-Launcher/{selected_version}")
        nwjs_path = os.path.join(nwjs_dir, "nwjs.app/Contents/MacOS/nwjs")

        run_with_rosetta_file = os.path.join(nwjs_dir, "run-with-rosetta")
        if os.path.exists(run_with_rosetta_file):
            subprocess.Popen(["arch", "-x86_64", nwjs_path, folder_path])
        else:
            subprocess.Popen([nwjs_path, folder_path])
        logging.info("Game launched using NWJS version %s.", selected_version)

    def install_nwjs(self):
        URL = "https://nwjs.io/versions"
        BASE_DIR = os.path.expanduser("~/Applications/RPGM-Launcher")

        def download_and_install(version, arch, use_rosetta=False):
            url = f"https://dl.nwjs.io/{version}/nwjs-sdk-{version}-osx-{arch}.zip"
            tmp_file = "/tmp/nwjs.zip"

            logging.info("Downloading and installing version %s for %s architecture...", version, arch)
            response = requests.get(url)
            if response.status_code != 200:
                logging.error("Failed to download version %s for %s.", version, arch)
                return

            with open(tmp_file, "wb") as f:
                f.write(response.content)

            subprocess.run(["unzip", "-q", tmp_file, "-d", "/tmp"])

            target_dir = os.path.join(BASE_DIR, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
            
            os.makedirs(target_dir, exist_ok=True)
            shutil.move(f"/tmp/nwjs-sdk-{version}-osx-{arch}/nwjs.app", target_dir)
            logging.info("Version %s installed successfully at %s", version, target_dir)

            if use_rosetta:
                with open(os.path.join(target_dir, "run-with-rosetta"), "w") as f:
                    f.write("This file indicates NWJS should be run with Rosetta 2.")

            os.remove(tmp_file)
            shutil.rmtree(f"/tmp/nwjs-sdk-{version}-osx-{arch}")

        arch = "arm64" if platform.machine() == "arm64" else "x64"
        logging.info("Querying available versions...")
        response = requests.get(URL)
        if response.status_code != 200:
            logging.error("Failed to query NWJS versions.")
            return

        data = response.json()
        versions = {v["version"]: v for v in data["versions"]}
        
        version, ok = self.show_version_selection_dialog(versions.keys())
        if ok and version:
            version_info = versions[version]
            use_rosetta = False
            if arch == "arm64" and "osx-arm64" not in version_info["files"]:
                use_rosetta = self.show_rosetta_warning()
                if not use_rosetta:
                    return
                arch = "x64"
            download_and_install(version, arch, use_rosetta)
            self.update_version_selector()
            if self.check_nwjs_installed():
                self.select_button.setEnabled(True)
                self.select_button.setText("Select Game Folder")
            else:
                self.select_button.setEnabled(False)
                self.select_button.setText("NWJS not installed")
            QMessageBox.information(self, "Install NWJS Version", "NWJS installation completed successfully.")

    def show_version_selection_dialog(self, versions):
        dialog = QDialog(self)
        dialog.setWindowTitle("Select NWJS Version")
        layout = QFormLayout(dialog)
        
        combo_box = QComboBox(dialog)
        combo_box.addItems(versions)
        layout.addRow("Version:", combo_box)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addRow(buttons)

        result = dialog.exec()
        return combo_box.currentText(), result == QDialog.Accepted

    def show_rosetta_warning(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("Rosetta Warning")
        layout = QVBoxLayout(dialog)

        label = QLabel("An Apple Silicon / ARM64 build of this version of NWJS is unavailable. Would you like to use the Intel / x64 version with Rosetta?", dialog)
        layout.addWidget(label)

        buttons = QDialogButtonBox(QDialogButtonBox.Yes | QDialogButtonBox.No, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        return result == QDialog.Accepted

    def uninstall_nwjs(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v))]

        version, ok = self.show_version_selection_dialog(versions)
        if ok and version:
            target_dir = os.path.join(applications_dir, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
                self.update_version_selector()
                self.update_select_button_state()
                logging.info("NWJS version %s uninstalled successfully.", version)
                QMessageBox.information(self, "Uninstall NWJS Version", f"NWJS version {version} uninstalled successfully.")
            else:
                logging.error("NWJS version %s not found.", version)
                QMessageBox.critical(self, "Error", f"NWJS version {version} not found.")

def check_appdir():
    app_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
    if not os.path.exists(app_dir):
        os.makedirs(app_dir)
    # Clear log file on startup
    with open(LOG_FILE, 'w') as file:
        file.write("")

def main():
    check_appdir()
    
    # Redirect stdout and stderr to log file
    sys.stdout = open(LOG_FILE, 'a')
    sys.stderr = open(LOG_FILE, 'a')
    
    app = QApplication(sys.argv)
    window = FolderPathApp()
    window.show()
    sys.exit(app.exec())

if __name__ == "__main__":
    main()
