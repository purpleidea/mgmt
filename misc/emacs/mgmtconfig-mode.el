;;; mgmtconfig-mode.el --- mgmt configuration management language

;; Copyright (C) 2013-2019+ James Shubin and the project contributors
;; Written by James Shubin <james@shubin.ca> and the project contributors
;;
;; This program is free software: you can redistribute it and/or modify
;; it under the terms of the GNU General Public License as published by
;; the Free Software Foundation, either version 3 of the License, or
;; (at your option) any later version.
;;
;; This program is distributed in the hope that it will be useful,
;; but WITHOUT ANY WARRANTY; without even the implied warranty of
;; MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
;; GNU General Public License for more details.
;;
;; You should have received a copy of the GNU General Public License
;; along with this program.  If not, see <http:;;www.gnu.org/licenses/>.

;; Author: Peter Oliver <mgmtconfig@mavit.org.uk>
;; Maintainer: Mgmt contributors <https://github.com/purpleidea/mgmt>
;; Keywords: languages
;; URL: https://github.com/purpleidea/mgmt/misc/emacs
;; Package-Requires: ((emacs "24.3"))

;;; Commentary:
;;
;; Major mode for editing the mgmt config language.  Mgmt is a
;; distributed, event-driven, parallel configuration management tool.
;;
;; https://github.com/purpleidea/mgmt

;;; Code:

(require 'smie)

(defconst mgmtconfig-smie-grammar
      (smie-prec2->grammar
       (smie-bnf->prec2
        '((map ("{" pairs "}"))
          (pairs (pair) (pair "," pairs))
          (pair (key "=>" value))
          (key)
          (value)))))

(defun mgmtconfig-smie-rules (method token)
  "Rules for indenting the mgmt language.
METHOD and TOKEN are as for `smie-rules-function'."
  (pcase (cons method token)
    ;; Statements in mgmt end at the end of a line.  This causes SMIE
    ;; not to indent every line as if it were a continuation of the
    ;; statement on the previous line:
    (`(:list-intro . ,_) t)))

(defconst mgmtconfig-mode-syntax-table
  (let ((st (make-syntax-table)))
    (modify-syntax-entry ?# "<" st)
    (modify-syntax-entry ?\n ">" st)
    st))

(defconst mgmtconfig-mode-highlights
  ;; Keywords and types cribbed from
  ;; https://github.com/purpleidea/mgmt/blob/master/lang/lexer.nex.
  ;; Resources from
  ;; https://github.com/purpleidea/mgmt/tree/master/resources.
  `((,(regexp-opt (list "if"
                        "else"
                        "in"
                        "true"
                        "false")
                  'words)
     . font-lock-keyword-face)
    (,(concat "^\\s-*" (regexp-opt (list "actions"
                                         "augeas"
                                         "autoedge"
                                         "autogroup"
                                         "aws_ec2"
                                         "edge"
                                         "exec"
                                         "file"
                                         "graph"
                                         "group"
                                         "hostname"
                                         "interfaces"
                                         "kv"
                                         "metaparams"
                                         "mgraph"
                                         "msg"
                                         "noop"
                                         "nspawn"
                                         "password"
                                         "pkg"
                                         "print"
                                         "refresh"
                                         "resources"
                                         "semaphore"
                                         "sendrecv"
                                         "svc"
                                         "timer"
                                         "uid"
                                         "user"
                                         "util"
                                         "virt")
                                   'words))
     . font-lock-builtin-face)
    (,(regexp-opt (list "bool"
                        "str"
                        "int"
                        "float"
                        "struct"
                        "variant")
                  'words)
     . font-lock-type-face)
    ("\\$\\w+" . font-lock-variable-name-face)))

;;;###autoload
(define-derived-mode mgmtconfig-mode prog-mode "mgmt"
  "Major mode for editing the mgmt config language."

  (setq-local comment-start "# ")
  (setq-local comment-start-skip "#+\\s-*")

  (setq font-lock-defaults '(mgmtconfig-mode-highlights))

  (smie-setup mgmtconfig-smie-grammar #'mgmtconfig-smie-rules))

;;;###autoload
(add-to-list 'auto-mode-alist '("\\.mcl\\'" . mgmtconfig-mode))

(provide 'mgmtconfig-mode)

;;; mgmtconfig-mode.el ends here
